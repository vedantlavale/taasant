package s3store

import (
	"context"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"slices"
	"strconv"
)

// keep is how many chunks stay in memory after a reader fetched them.
const keep = 4

// fetched is a chunk that is on its way or has arrived.
type fetched struct {
	done  chan struct{} // closed once data or err is set
	data  []byte
	err   error
	ready bool // the fetch is over, so this entry may be evicted
}

// reader reads an object without downloading all of it. Seek only notes the
// position, and a Read fetches the one chunk it lands in.
//
// The readers of a Store share what they fetch. Finder asks for one file in
// several requests at once, each with a reader of its own, and this way a
// chunk is still downloaded only once.
type reader struct {
	s      *Store
	ctx    context.Context
	aead   cipher.AEAD
	who    string // the key, for the shared cache
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
		sum := sha256.Sum256(key)
		r.who = hex.EncodeToString(sum[:8])
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
		data, err := r.s.chunk(r.ctx, r.aead, r.who, r.id, index, r.size)
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

// chunk returns one decrypted chunk. The last few are kept, and a chunk that
// another reader is fetching right now is waited for, not fetched again. The
// cache is told apart by key too: what was fetched and opened with one key
// must never be handed out as the plaintext for another.
func (s *Store) chunk(ctx context.Context, aead cipher.AEAD, who, id string, index int, size int64) ([]byte, error) {
	name := id + "/" + strconv.Itoa(index) + "@" + who
	s.mu.Lock()
	f, found := s.chunks[name]
	if !found {
		f = &fetched{done: make(chan struct{})}
		if s.chunks == nil {
			s.chunks = map[string]*fetched{}
		}
		s.chunks[name] = f
		s.order = append(s.order, name)
		// Only finished chunks are evicted. One that is still being fetched
		// must stay registered, or another reader starts a second download.
		for len(s.order) > keep {
			i := slices.IndexFunc(s.order, func(other string) bool { return s.chunks[other].ready })
			if i < 0 {
				break
			}
			delete(s.chunks, s.order[i])
			s.order = slices.Delete(s.order, i, i+1)
		}
	}
	s.mu.Unlock()

	if !found {
		f.data, f.err = s.fetchChunk(ctx, aead, id, index, size)
		s.mu.Lock()
		f.ready = true
		if f.err != nil {
			// A failure is not kept: the next reader tries again.
			if s.chunks[name] == f {
				delete(s.chunks, name)
				s.order = slices.DeleteFunc(s.order, func(other string) bool { return other == name })
			}
		}
		s.mu.Unlock()
		close(f.done)
		return f.data, f.err
	}

	select {
	case <-f.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if f.err != nil {
		// The reader that was fetching it may just have been stopped.
		return s.fetchChunk(ctx, aead, id, index, size)
	}
	return f.data, nil
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
