package s3store

import (
	"context"
	"crypto/cipher"
	"fmt"
	"io"
	"io/fs"
	"net/http"
)

// reader reads an object without downloading all of it. Seek only notes the
// position, and a Read fetches the one chunk it lands in.
type reader struct {
	s      *Store
	ctx    context.Context
	aead   cipher.AEAD
	err    error
	id     string
	size   int64
	offset int64

	index int // the chunk that data holds
	data  []byte
}

// Open returns a reader for the object stored under id, which is size bytes
// long. Nothing is fetched until the first Read.
func (s *Store) Open(ctx context.Context, key []byte, id string, size int64) io.ReadSeeker {
	r := &reader{s: s, ctx: ctx, id: id, size: size}
	if r.aead, r.err = newCipher(key); r.err == nil {
		r.err = checkName(id)
	}
	return r
}

func (r *reader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	if r.offset >= r.size {
		return 0, io.EOF
	}
	index := int(r.offset / int64(r.s.chunkSize))
	if r.data == nil || index != r.index {
		data, err := r.s.fetchChunk(r.ctx, r.aead, r.id, index, r.size)
		if err != nil {
			return 0, err
		}
		r.index, r.data = index, data
	}
	start := r.offset - int64(index)*int64(r.s.chunkSize)
	if start >= int64(len(r.data)) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[start:])
	r.offset += int64(n)
	return n, nil
}

func (r *reader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		offset += r.offset
	case io.SeekEnd:
		offset += r.size
	}
	if offset < 0 {
		return 0, fs.ErrInvalid
	}
	r.offset = offset
	return offset, nil
}

// fetchChunk gets one chunk of an object that holds size bytes. Where a chunk
// lies follows from its number, so no list of them is needed.
func (s *Store) fetchChunk(ctx context.Context, aead cipher.AEAD, id string, index int, size int64) ([]byte, error) {
	chunks := max(1, int((size+int64(s.chunkSize)-1)/int64(s.chunkSize)))
	sealed := int64(s.chunkSize + overhead)
	from := int64(index) * sealed
	to := min(from+sealed, size+int64(chunks)*overhead) - 1

	res, err := s.do(ctx, request{
		method: http.MethodGet, object: id,
		header: map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", from, to)},
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != to-from+1 {
		return nil, io.ErrUnexpectedEOF
	}
	return openChunk(aead, data, id, index, index == chunks-1)
}
