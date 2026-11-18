package s3store

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxAttempts = 4

// attemptTimeout is how long one request may take. A chunk is 16 MiB, so this
// allows a slow line, and no more: a connection that has gone quiet in the
// middle of a body would otherwise never give up. A download of a whole
// object has no limit, as how long it takes depends on how big it is.
const attemptTimeout = 10 * time.Minute

// retryWait is the pause before trying again after a broken connection.
var retryWait = 500 * time.Millisecond

// request is one call to S3. Body is sent as it is, so a call can be repeated.
type request struct {
	method string
	object string
	query  url.Values
	body   []byte
	header map[string]string
	// sent, if not nil, is told how much of the body has gone out so far.
	// After a failed attempt the count starts again from zero.
	sent func(done, total int)
}

// do sends a request and tries again when that can help: the connection
// broke, or S3 is busy. The caller closes the body of what it gets back.
func (s *Store) do(ctx context.Context, r request) (*http.Response, error) {
	target := strings.TrimRight(s.Endpoint, "/") + "/" + s.Bucket
	if r.object != "" {
		target += "/" + r.object
	}

	for attempt := 1; ; attempt++ {
		var reader io.Reader = bytes.NewReader(r.body)
		if r.sent != nil {
			reader = &countingReader{reader: reader, total: len(r.body), report: r.sent}
		}
		attemptCtx, cancel := ctx, context.CancelFunc(func() {})
		if r.method != http.MethodGet || r.header["Range"] != "" {
			attemptCtx, cancel = context.WithTimeout(ctx, attemptTimeout)
		}
		req, err := http.NewRequestWithContext(attemptCtx, r.method, target, reader)
		if err != nil {
			cancel()
			return nil, err
		}
		req.URL.RawQuery = r.query.Encode()
		req.ContentLength = int64(len(r.body))
		for name, value := range r.header {
			req.Header.Set(name, value)
		}
		s.sign(req, r.body, time.Now())

		res, err := s.Client.Do(req)
		if err == nil && res.StatusCode < 300 {
			// The timeout must outlive the body that is still to be read.
			res.Body = &cancelOnClose{ReadCloser: res.Body, cancel: cancel}
			return res, nil
		}
		if err == nil {
			err = readError(res)
		}
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var failed *apiError
		retryable := !errors.As(err, &failed) || failed.status == http.StatusTooManyRequests || failed.status >= 500
		if !retryable || attempt == maxAttempts {
			return nil, fmt.Errorf("s3 %s: %w", r.method, withoutURL(err))
		}
		select {
		case <-time.After(retryWait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// cancelOnClose ends a request's timeout once its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}

// apiError is what S3 says when it refuses a request.
type apiError struct {
	status  int
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func (e *apiError) Error() string {
	if e.Code == "" {
		return http.StatusText(e.status)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func readError(res *http.Response) error {
	defer res.Body.Close()
	e := &apiError{status: res.StatusCode}
	xml.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(e)
	return e
}

// withoutURL drops the address from a network error. It is long and says
// nothing that the error does not.
func withoutURL(err error) error {
	var failed *url.Error
	if errors.As(err, &failed) {
		return failed.Err
	}
	return err
}

// countingReader reports how much has been read through it.
type countingReader struct {
	reader      io.Reader
	done, total int
	report      func(done, total int)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.done += n
	c.report(c.done, c.total)
	return n, err
}
