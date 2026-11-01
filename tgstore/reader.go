package tgstore

import (
	"context"
	"crypto/cipher"
	"io"
	"io/fs"
	"slices"
)

// keep is how many parts stay in memory after a Reader fetched them.
const keep = 4

// Reader reads a stored object from any position. A part is downloaded when
// a Read lands in it.
//
// The Readers of a Store share what they fetch. Finder asks for one file in
// several requests at once, each with a Reader of its own, and this way a
// part is still downloaded only once.
type Reader struct {
	store  *Store
	ctx    context.Context
	key    []byte
	id     string
	size   int64
	offset int64

	aead  cipher.AEAD
	shape shape
	index int // the part that data holds
	data  []byte
}

// shape is what a Reader has to know about an object: its parts, and how
// long each one but the last is. That length says which part a position is in.
type shape struct {
	parts    []stored
	partSize int64
}

// fetched is a part that is on its way from Telegram or has arrived.
type fetched struct {
	done chan struct{} // closed once data or err is set
	data []byte
	err  error
}

// Open returns a Reader for the object stored under id, which is size bytes
// long. Telegram is not asked for anything until the first Read.
func (s *Store) Open(ctx context.Context, key []byte, id string, size int64) io.ReadSeeker {
	return &Reader{store: s, ctx: ctx, key: key, id: id, size: size}
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.aead == nil {
		aead, err := newCipher(r.key)
		if err != nil {
			return 0, err
		}
		shape, err := r.store.shapeOf(r.ctx, aead, r.id)
		if err != nil {
			return 0, err
		}
		r.aead, r.shape = aead, shape
	}

	index := int(r.offset / r.shape.partSize)
	if r.data == nil || index != r.index {
		if index >= len(r.shape.parts) {
			return 0, io.ErrUnexpectedEOF
		}
		data, err := r.store.part(r.ctx, r.aead, r.shape.parts[index].FileID)
		if err != nil {
			return 0, err
		}
		r.index, r.data = index, data
	}

	start := r.offset - int64(index)*r.shape.partSize
	if start >= int64(len(r.data)) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[start:])
	r.offset += int64(n)
	return n, nil
}

// Seek only notes the position. The next Read fetches what it needs.
func (r *Reader) Seek(offset int64, whence int) (int64, error) {
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

// shapeOf returns the shape of an object, and remembers it. The length of a
// part is not written down anywhere, so the first part is fetched to see it.
func (s *Store) shapeOf(ctx context.Context, aead cipher.AEAD, id string) (shape, error) {
	s.mu.Lock()
	known, found := s.shapes[id]
	s.mu.Unlock()
	if found {
		return known, nil
	}

	m, _, err := s.fetchManifest(ctx, aead, id)
	if err != nil {
		return shape{}, err
	}
	if len(m.Parts) == 0 {
		return shape{}, io.ErrUnexpectedEOF
	}
	first, err := s.part(ctx, aead, m.Parts[0].FileID)
	if err != nil {
		return shape{}, err
	}
	if len(first) == 0 {
		return shape{}, io.ErrUnexpectedEOF
	}
	known = shape{parts: m.Parts, partSize: int64(len(first))}

	s.mu.Lock()
	if s.shapes == nil {
		s.shapes = map[string]shape{}
	}
	s.shapes[id] = known
	s.mu.Unlock()
	return known, nil
}

// part returns one decrypted part. The last few are kept, and a part that
// another Reader is fetching right now is waited for, not fetched again.
func (s *Store) part(ctx context.Context, aead cipher.AEAD, fileID string) ([]byte, error) {
	s.mu.Lock()
	f, found := s.parts[fileID]
	if !found {
		f = &fetched{done: make(chan struct{})}
		if s.parts == nil {
			s.parts = map[string]*fetched{}
		}
		s.parts[fileID] = f
		s.order = append(s.order, fileID)
		if len(s.order) > keep {
			delete(s.parts, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.mu.Unlock()

	if !found {
		f.data, f.err = s.fetchPart(ctx, aead, fileID, nil)
		if f.err != nil {
			// A failure is not kept: the next Reader tries again.
			s.mu.Lock()
			if s.parts[fileID] == f {
				delete(s.parts, fileID)
				s.order = slices.DeleteFunc(s.order, func(id string) bool { return id == fileID })
			}
			s.mu.Unlock()
		}
		close(f.done)
		return f.data, f.err
	}

	select {
	case <-f.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if f.err != nil {
		// The Reader that was fetching it may just have been stopped.
		return s.fetchPart(ctx, aead, fileID, nil)
	}
	return f.data, nil
}
