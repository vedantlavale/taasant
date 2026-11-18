package s3store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testChunk = 1024

// fakeS3 keeps objects in memory and answers the few requests the store makes.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	// uploads are the multipart uploads that are not finished: part by part.
	uploads map[string]map[int][]byte
	// fail, if not nil, may turn a request down with a status.
	fail func(r *http.Request) int
	// requests counts what arrived, ranges those that asked for a range.
	requests, ranges int
}

func newFake(t *testing.T) (*Store, *fakeS3) {
	t.Helper()
	retryWait = 0
	fake := &fakeS3{objects: map[string][]byte{}, uploads: map[string]map[int][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	store := New(server.URL, "auto", "bucket", "ACCESS", "SECRET")
	store.chunkSize = testChunk
	return store, fake
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++

	body, _ := io.ReadAll(r.Body)
	sum := sha256.Sum256(body)
	switch {
	case !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=ACCESS/"):
		deny(w, 403, "AccessDenied", "no signature")
		return
	case r.Header.Get("x-amz-content-sha256") != hex.EncodeToString(sum[:]):
		deny(w, 400, "XAmzContentSHA256Mismatch", "the body is not what was signed")
		return
	case !strings.HasPrefix(r.URL.Path, "/bucket/"):
		deny(w, 404, "NoSuchBucket", "no such bucket")
		return
	}
	if f.fail != nil {
		if status := f.fail(r); status != 0 {
			deny(w, status, "SlowDown", "try later")
			return
		}
	}

	name, query := strings.TrimPrefix(r.URL.Path, "/bucket/"), r.URL.Query()
	switch {
	case r.Method == "POST" && query.Has("uploads"):
		id := "upload-" + strconv.Itoa(len(f.uploads)+1)
		f.uploads[id] = map[int][]byte{}
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", id)

	case r.Method == "PUT" && query.Has("uploadId"):
		part, _ := strconv.Atoi(query.Get("partNumber"))
		parts, found := f.uploads[query.Get("uploadId")]
		if !found {
			deny(w, 404, "NoSuchUpload", "gone")
			return
		}
		parts[part] = body
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(part)+`"`)

	case r.Method == "POST" && query.Has("uploadId"):
		parts, found := f.uploads[query.Get("uploadId")]
		var complete struct {
			Parts []struct {
				PartNumber int
				ETag       string
			} `xml:"Part"`
		}
		if err := xml.Unmarshal(body, &complete); err != nil || !found {
			deny(w, 400, "MalformedXML", "bad")
			return
		}
		var whole []byte
		for i, part := range complete.Parts {
			if part.PartNumber != i+1 || part.ETag != `"etag-`+strconv.Itoa(i+1)+`"` {
				deny(w, 400, "InvalidPart", "wrong list")
				return
			}
			whole = append(whole, parts[part.PartNumber]...)
		}
		f.objects[name] = whole
		delete(f.uploads, query.Get("uploadId"))
		io.WriteString(w, "<CompleteMultipartUploadResult></CompleteMultipartUploadResult>")

	case r.Method == "DELETE" && query.Has("uploadId"):
		delete(f.uploads, query.Get("uploadId"))
		w.WriteHeader(204)

	case r.Method == "PUT":
		f.objects[name] = body

	case r.Method == "DELETE":
		delete(f.objects, name)
		w.WriteHeader(204)

	case r.Method == "GET":
		data, found := f.objects[name]
		if !found {
			deny(w, 404, "NoSuchKey", "no such object")
			return
		}
		if header := r.Header.Get("Range"); header != "" {
			f.ranges++
			var from, to int
			fmt.Sscanf(header, "bytes=%d-%d", &from, &to)
			if to >= len(data) {
				to = len(data) - 1
			}
			w.WriteHeader(206)
			w.Write(data[from : to+1])
			return
		}
		w.Write(data)
	}
}

func deny(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message></Error>", code, message)
}

func randomBytes(n int) []byte {
	data := make([]byte, n)
	rand.Read(data)
	return data
}

func TestRoundTrip(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	// Around the edges of a chunk, and over several.
	for _, size := range []int{0, 1, testChunk - 1, testChunk, testChunk + 1, 3*testChunk + 5, 4 * testChunk} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			content := randomBytes(size)
			var sent, received int
			id, err := store.Upload(context.Background(), key, bytes.NewReader(content), func(n int) { sent += n })
			if err != nil {
				t.Fatal(err)
			}
			if sent != size {
				t.Errorf("upload reported %d bytes, want %d", sent, size)
			}
			var got bytes.Buffer
			if err := store.Download(context.Background(), key, id, &got, func(n int) { received += n }); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), content) {
				t.Error("what came back is not what went in")
			}
			if received != size {
				t.Errorf("download reported %d bytes, want %d", received, size)
			}
			if bytes.Contains(fake.objects[id], content[:min(size, 64)]) && size > 0 {
				t.Error("the object holds the file as it is")
			}
			if len(fake.uploads) != 0 {
				t.Error("an upload was left open")
			}
		})
	}
}

func TestWrongKey(t *testing.T) {
	store, _ := newFake(t)
	id, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(100)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Download(context.Background(), randomBytes(32), id, io.Discard, nil); err == nil {
		t.Error("a wrong key opened the file")
	}
}

// Whoever can write to the bucket must not be able to change a file without
// the download noticing.
func TestChangedObject(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	sealed := testChunk + overhead
	upload := func() string {
		id, err := store.Upload(context.Background(), key, bytes.NewReader(randomBytes(3*testChunk+10)), nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	tests := map[string]func(data []byte) []byte{
		"cut after a chunk": func(d []byte) []byte { return d[:sealed] },
		"last chunk gone":   func(d []byte) []byte { return d[:3*sealed] },
		"chunks swapped": func(d []byte) []byte {
			out := bytes.Clone(d)
			copy(out[:sealed], d[sealed:2*sealed])
			copy(out[sealed:2*sealed], d[:sealed])
			return out
		},
		"a chunk repeated": func(d []byte) []byte { return append(bytes.Clone(d[:sealed]), d...) },
		"a byte changed":   func(d []byte) []byte { out := bytes.Clone(d); out[sealed+40] ^= 1; return out },
		"nothing":          func(d []byte) []byte { return nil },
	}
	for name, change := range tests {
		id := upload()
		fake.objects[id] = change(fake.objects[id])
		if err := store.Download(context.Background(), key, id, io.Discard, nil); err == nil {
			t.Errorf("%s: the download did not notice", name)
		}
	}

	// An object cannot be passed off as another one either.
	a, b := upload(), upload()
	fake.objects[a] = fake.objects[b]
	if err := store.Download(context.Background(), key, a, io.Discard, nil); err == nil {
		t.Error("an object under another name was accepted")
	}
}

func TestOpen(t *testing.T) {
	store, _ := newFake(t)
	key, content := randomBytes(32), randomBytes(3*testChunk+100)
	id, err := store.Upload(context.Background(), key, bytes.NewReader(content), nil)
	if err != nil {
		t.Fatal(err)
	}
	size := int64(len(content))

	got, err := io.ReadAll(store.Open(context.Background(), key, id, size))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("reading it all gave other bytes, or %v", err)
	}

	r := store.Open(context.Background(), key, id, size)
	for _, offset := range []int64{testChunk - 3, 2*testChunk + 1, 0, size - 10} {
		r.Seek(offset, io.SeekStart)
		got := make([]byte, 20)
		n, err := io.ReadFull(r, got)
		if err != nil && err != io.ErrUnexpectedEOF {
			t.Fatal(err)
		}
		if !bytes.Equal(got[:n], content[offset:min(offset+20, size)]) {
			t.Errorf("reading from %d gave other bytes", offset)
		}
	}
	if end, _ := r.Seek(-5, io.SeekEnd); end != size-5 {
		t.Errorf("seeking from the end went to %d", end)
	}

	missing := store.Open(context.Background(), key, newName(), 10)
	if _, err := missing.Read(make([]byte, 1)); err == nil {
		t.Error("reading an object that is not there worked")
	}
	if _, err := store.Open(context.Background(), key, "../x", 10).Read(make([]byte, 1)); err == nil {
		t.Error("an ID that is not a name was used")
	}
}

func TestDelete(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	id, _ := store.Upload(context.Background(), key, bytes.NewReader(randomBytes(10)), nil)
	if err := store.Delete(context.Background(), key, id, nil); err != nil || len(fake.objects) != 0 {
		t.Errorf("delete: %v, %d objects left", err, len(fake.objects))
	}
	if err := store.Delete(context.Background(), key, "no", nil); err == nil {
		t.Error("an invalid ID was deleted")
	}
}

func TestBusyS3IsRetried(t *testing.T) {
	store, fake := newFake(t)
	busy := 2
	fake.fail = func(r *http.Request) int {
		if r.Method == "PUT" && busy > 0 {
			busy--
			return 503
		}
		return 0
	}
	if _, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(100)), nil); err != nil {
		t.Fatalf("two busy answers broke the upload: %v", err)
	}
}

func TestRefusalIsNotRetried(t *testing.T) {
	store, fake := newFake(t)
	fake.fail = func(r *http.Request) int { return 403 }
	_, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(100)), nil)
	if err == nil || !strings.Contains(err.Error(), "SlowDown") {
		t.Fatalf("a refusal gave %v", err)
	}
	if fake.requests != 1 {
		t.Errorf("a refusal was sent %d times", fake.requests)
	}
}

// A failed upload must not leave parts in the bucket, which would be paid for.
func TestFailedUploadIsAborted(t *testing.T) {
	store, fake := newFake(t)
	fake.fail = func(r *http.Request) int {
		if r.URL.Query().Get("partNumber") == "3" {
			return 403
		}
		return 0
	}
	_, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(5*testChunk)), nil)
	if err == nil {
		t.Fatal("the upload worked although a part was refused")
	}
	if len(fake.uploads) != 0 || len(fake.objects) != 0 {
		t.Errorf("%d uploads and %d objects were left", len(fake.uploads), len(fake.objects))
	}
}

func TestCancelledUploadIsAborted(t *testing.T) {
	store, fake := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	reader := io.MultiReader(bytes.NewReader(randomBytes(3*testChunk)), readerFunc(func([]byte) (int, error) {
		cancel()
		return 0, ctx.Err()
	}))
	if _, err := store.Upload(ctx, randomBytes(32), reader, nil); err == nil {
		t.Fatal("a cancelled upload worked")
	}
	if len(fake.uploads) != 0 {
		t.Error("a cancelled upload was left open")
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestCheck(t *testing.T) {
	store, fake := newFake(t)
	if err := store.Check(context.Background()); err != nil || len(fake.objects) != 0 {
		t.Errorf("check: %v, %d objects left", err, len(fake.objects))
	}
	store.Bucket = "other"
	err := store.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "NoSuchBucket") {
		t.Errorf("a missing bucket gave %v", err)
	}
	store.Bucket, store.AccessKey = "bucket", ""
	if err := store.Check(context.Background()); err == nil {
		t.Error("a request without keys was accepted")
	}
}

func TestUploadProgressMoves(t *testing.T) {
	store, _ := newFake(t)
	calls := 0
	if _, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(testChunk)), func(int) { calls++ }); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Errorf("progress was reported %d times", calls)
	}
}

func TestSign(t *testing.T) {
	// The example in Amazon's documentation of Signature Version 4.
	s := &Store{Region: "us-east-1", AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, nil, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization is\n%s\nwant\n%s", got, want)
	}
}

func TestSignQuery(t *testing.T) {
	// The second example in the same documentation, a query without a value.
	s := &Store{Region: "us-east-1", AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	u, _ := url.Parse("https://examplebucket.s3.amazonaws.com/?lifecycle")
	req := &http.Request{Method: "GET", URL: u, Header: http.Header{}}
	s.sign(req, nil, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "Signature=fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"
	if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, want) {
		t.Errorf("Authorization is %s, want it to end with %s", got, want)
	}
}

// Finder reads one file through several readers at once. Together they must
// not download a chunk more than once.
func TestReadersShare(t *testing.T) {
	store, fake := newFake(t)
	key, content := randomBytes(32), randomBytes(4*testChunk)
	id, err := store.Upload(context.Background(), key, bytes.NewReader(content), nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := io.ReadAll(store.Open(context.Background(), key, id, int64(len(content))))
			if err != nil || !bytes.Equal(got, content) {
				t.Errorf("a reader got other bytes, or %v", err)
			}
		})
	}
	wg.Wait()
	if fake.ranges != 4 {
		t.Fatalf("8 readers made %d downloads of 4 chunks", fake.ranges)
	}

	// Everything is in memory, so a reader later needs nothing.
	got, err := io.ReadAll(store.Open(context.Background(), key, id, int64(len(content))))
	if err != nil || !bytes.Equal(got, content) || fake.ranges != 4 {
		t.Fatalf("a later reader made %d more downloads, or %v", fake.ranges-4, err)
	}
}
