package s3store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"
)

// sign adds what S3 wants to see on a request: the hash of the body, the time
// and a signature made with the secret key (AWS Signature Version 4).
func (s *Store) sign(req *http.Request, body []byte, now time.Time) {
	payload := hash(body)
	stamp := now.UTC().Format("20060102T150405Z")
	req.Header.Set("x-amz-content-sha256", payload)
	req.Header.Set("x-amz-date", stamp)

	// The headers that are signed: the host, the range and everything x-amz.
	names := []string{"host"}
	for name := range req.Header {
		lower := strings.ToLower(name)
		if lower == "range" || strings.HasPrefix(lower, "x-amz-") {
			names = append(names, lower)
		}
	}
	slices.Sort(names)
	var headers strings.Builder
	for _, name := range names {
		value := req.URL.Host
		if name != "host" {
			value = strings.TrimSpace(req.Header.Get(name))
		}
		headers.WriteString(name + ":" + value + "\n")
	}
	signed := strings.Join(names, ";")

	// S3 wants the space in a query written as %20, not as +.
	query := strings.ReplaceAll(req.URL.Query().Encode(), "+", "%20")
	req.URL.RawQuery = query
	request := strings.Join([]string{req.Method, req.URL.EscapedPath(), query, headers.String(), signed, payload}, "\n")

	scope := stamp[:8] + "/" + s.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hash([]byte(request))
	secret := mac(mac(mac(mac([]byte("AWS4"+s.SecretKey), stamp[:8]), s.Region), "s3"), "aws4_request")
	signature := hex.EncodeToString(mac(secret, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+signature)
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
