// Package s3store stores encrypted files in a bucket that speaks the S3 API:
// AWS S3, Cloudflare R2, MinIO, Backblaze B2 and others.
//
// A file is one object with a random name. It is cut into chunks of a fixed
// size, each sealed on its own, so any chunk can be fetched by itself with a
// ranged request and a file never has to be in memory as a whole. Chunks are
// the parts of a multipart upload, which is why they are all the same size
// but the last: R2 insists on that.
package s3store

import (
	"bufio"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// chunkSize is how much of a file one chunk holds. It is part of how an
// object is laid out: change it and every object stored before is unreadable.
// S3 wants parts of at least 5 MiB and allows 10000 of them, which makes
// 156 GiB the biggest file.
const (
	chunkSize = 16 << 20
	maxParts  = 10000
	// overhead is how much longer a sealed chunk is than the bytes in it.
	overhead = 12 + 16
)

// Store must be created with New.
type Store struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Workers   int
	Client    *http.Client

	chunkSize int
}

func New(endpoint, region, bucket, accessKey, secretKey string) *Store {
	return &Store{
		Endpoint: endpoint, Region: region, Bucket: bucket,
		AccessKey: accessKey, SecretKey: secretKey,
		Workers:   3,
		Client:    http.DefaultClient,
		chunkSize: chunkSize,
	}
}

// Check tries the bucket with a small object, so a wrong key or a missing
// bucket is found when someone sets up, not at their first upload.
func (s *Store) Check(ctx context.Context) error {
	name := newName()
	res, err := s.do(ctx, request{method: http.MethodPut, object: name, body: []byte("taasant")})
	if err != nil {
		return err
	}
	res.Body.Close()
	return s.Delete(ctx, nil, name)
}

func newName() string {
	name := make([]byte, 16)
	rand.Read(name)
	return hex.EncodeToString(name)
}

// checkName refuses an ID that is not one of ours, as it ends up in a URL.
func checkName(id string) error {
	if raw, err := hex.DecodeString(id); err != nil || len(raw) != 16 {
		return errors.New("invalid object id")
	}
	return nil
}

type chunk struct {
	index int
	data  []byte
	last  bool
}

// chunker reads in chunks, always one ahead, to know which one is the last.
type chunker struct {
	r    io.Reader
	size int
	next []byte
	eof  bool
	err  error
}

func newChunker(r io.Reader, size int) *chunker {
	c := &chunker{r: r, size: size}
	c.fill()
	return c
}

func (c *chunker) fill() {
	buf := make([]byte, c.size)
	n, err := io.ReadFull(c.r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		c.eof, err = true, nil
	}
	c.next, c.err = buf[:n], err
}

// take returns the next chunk and whether it is the last. Even an empty
// file has a chunk, an empty one.
func (c *chunker) take() ([]byte, bool, error) {
	data, err := c.next, c.err
	if err != nil {
		return nil, false, err
	}
	if c.eof {
		return data, true, nil
	}
	c.fill()
	return data, c.err == nil && c.eof && len(c.next) == 0, nil
}

// uploading is the state of one Upload.
type uploading struct {
	s        *Store
	aead     cipher.AEAD
	name     string
	progress func(bytes int)
	mu       sync.Mutex
}

// reporter returns what a request sending a chunk of plain bytes calls as the
// bytes go out. What goes out is sealed and a little larger, so it settles on
// the real size.
func (u *uploading) reporter(plain int) func(done, total int) {
	counted := 0
	return func(done, total int) {
		now := int(int64(done) * int64(plain) / int64(max(total, 1)))
		u.mu.Lock()
		if u.progress != nil {
			u.progress(now - counted)
		}
		counted = now
		u.mu.Unlock()
	}
}

// put sends one chunk, as the whole object or, with an uploadID, as a part of
// it. It returns the ETag S3 gave the part.
func (u *uploading) put(ctx context.Context, c chunk, uploadID string) (string, error) {
	report := u.reporter(len(c.data))
	r := request{
		method: http.MethodPut, object: u.name, sent: report,
		body: sealChunk(u.aead, c.data, u.name, c.index, c.last),
	}
	if uploadID != "" {
		r.query = url.Values{"partNumber": {strconv.Itoa(c.index + 1)}, "uploadId": {uploadID}}
	}
	res, err := u.s.do(ctx, r)
	if err != nil {
		return "", err
	}
	res.Body.Close()
	report(1, 1)
	return res.Header.Get("ETag"), nil
}

// Upload stores everything read from r and returns the ID needed to download it.
// The key must be 16, 24 or 32 bytes long.
//
// progress, if not nil, is called with a number of bytes each time some more
// of the object has been uploaded. The numbers add up to the size of the
// object. One can be negative: after a failed attempt a chunk starts again
// from zero. Calls never overlap.
func (s *Store) Upload(ctx context.Context, key []byte, r io.Reader, progress func(bytes int)) (string, error) {
	aead, err := newCipher(key)
	if err != nil {
		return "", err
	}
	u := &uploading{s: s, aead: aead, name: newName(), progress: progress}

	chunks := newChunker(r, s.chunkSize)
	data, last, err := chunks.take()
	if err != nil {
		return "", err
	}
	if last {
		_, err := u.put(ctx, chunk{0, data, true}, "")
		return u.name, err
	}

	uploadID, err := s.startUpload(ctx, u.name)
	if err != nil {
		return "", err
	}
	etags, err := u.putAll(ctx, chunk{0, data, false}, chunks, uploadID)
	if err == nil {
		err = s.finishUpload(ctx, u.name, uploadID, etags)
	}
	if err != nil {
		// Parts that are left in the bucket are kept, and paid for.
		s.abortUpload(u.name, uploadID)
		return "", err
	}
	return u.name, nil
}

// putAll sends first and the chunks after it, a few at a time. It returns
// the ETag of each part, in order.
func (u *uploading) putAll(ctx context.Context, first chunk, rest *chunker, uploadID string) ([]string, error) {
	outer := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		etags    = map[int]string{}
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}

	chunks := make(chan chunk)
	for range max(u.s.Workers, 1) {
		wg.Go(func() {
			for c := range chunks {
				etag, err := u.put(ctx, c, uploadID)
				if err != nil {
					fail(err)
					continue
				}
				mu.Lock()
				etags[c.index] = etag
				mu.Unlock()
			}
		})
	}

	for c := first; ctx.Err() == nil; {
		if c.index >= maxParts {
			fail(errors.New("the file is too big for one object, the most is 156 GiB"))
			break
		}
		chunks <- c
		if c.last {
			break
		}
		data, last, err := rest.take()
		if err != nil {
			fail(err)
			break
		}
		c = chunk{c.index + 1, data, last}
	}
	close(chunks)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := outer.Err(); err != nil {
		return nil, err
	}
	ordered := make([]string, len(etags))
	for i := range ordered {
		ordered[i] = etags[i]
	}
	return ordered, nil
}

func (s *Store) startUpload(ctx context.Context, name string) (string, error) {
	res, err := s.do(ctx, request{method: http.MethodPost, object: name, query: url.Values{"uploads": {""}}})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var started struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(res.Body).Decode(&started); err != nil || started.UploadID == "" {
		return "", errors.New("s3 did not start the upload")
	}
	return started.UploadID, nil
}

func (s *Store) finishUpload(ctx context.Context, name, uploadID string, etags []string) error {
	type part struct {
		PartNumber int
		ETag       string
	}
	var body struct {
		XMLName xml.Name `xml:"CompleteMultipartUpload"`
		Parts   []part   `xml:"Part"`
	}
	for i, etag := range etags {
		body.Parts = append(body.Parts, part{i + 1, etag})
	}
	data, err := xml.Marshal(body)
	if err != nil {
		return err
	}
	res, err := s.do(ctx, request{method: http.MethodPost, object: name, query: url.Values{"uploadId": {uploadID}}, body: data})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// S3 can fail even after it has said 200, and says so in the body.
	var answer struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	xml.NewDecoder(res.Body).Decode(&answer)
	if answer.Code != "" {
		return fmt.Errorf("s3 finishing the upload: %s: %s", answer.Code, answer.Message)
	}
	return nil
}

// abortUpload does not use the context of the upload, which is probably
// cancelled by now, and is not worried about failing: there is nobody to tell.
func (s *Store) abortUpload(name, uploadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := s.do(ctx, request{method: http.MethodDelete, object: name, query: url.Values{"uploadId": {uploadID}}}); err == nil {
		res.Body.Close()
	}
}

// Download writes the object stored under id to w. progress works as it does
// for Upload, with the bytes downloaded.
func (s *Store) Download(ctx context.Context, key []byte, id string, w io.Writer, progress func(bytes int)) error {
	aead, err := newCipher(key)
	if err != nil {
		return err
	}
	if err := checkName(id); err != nil {
		return err
	}
	res, err := s.do(ctx, request{method: http.MethodGet, object: id})
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// What arrives is sealed and a little larger than what it holds.
	raw, reported := 0, 0
	body := bufio.NewReaderSize(&meter{res.Body, func(n int) {
		raw += n
		now := int(int64(raw) * int64(s.chunkSize) / int64(s.chunkSize+overhead))
		if progress != nil {
			progress(now - reported)
		}
		reported = now
	}}, 64<<10)

	buf := make([]byte, s.chunkSize+overhead)
	total := 0
	for index := 0; ; index++ {
		n, err := io.ReadFull(body, buf)
		last := err == io.ErrUnexpectedEOF
		switch {
		case err == io.EOF:
			return errors.New("the stored file is cut short")
		case err == nil:
			_, err = body.Peek(1)
			last = err == io.EOF
			if err != nil && !last {
				return err
			}
		case !last:
			return err
		}
		plain, err := openChunk(aead, buf[:n], id, index, last)
		if err != nil {
			return err
		}
		if _, err := w.Write(plain); err != nil {
			return err
		}
		total += len(plain)
		if last {
			break
		}
	}
	if progress != nil {
		progress(total - reported)
	}
	return nil
}

// meter tells how much has been read through it.
type meter struct {
	r    io.Reader
	read func(n int)
}

func (m *meter) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	m.read(n)
	return n, err
}

// Delete removes the object stored under id. The key is not needed, there
// is only one object to remove.
func (s *Store) Delete(ctx context.Context, key []byte, id string) error {
	if err := checkName(id); err != nil {
		return err
	}
	res, err := s.do(ctx, request{method: http.MethodDelete, object: id})
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}
