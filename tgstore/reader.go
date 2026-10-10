package tgstore

import (
	"context"
	"crypto/cipher"
	"io"
	"io/fs"
)

// Reader reads a stored object from any position. A part is downloaded when
// a Read lands in it, and the last one is kept in memory.
type Reader struct {
	store  *Store
	ctx    context.Context
	key    []byte
	id     string
	size   int64
	offset int64

	aead     cipher.AEAD
	parts    []stored
	partSize int64
	index    int // the part that data holds
	data     []byte
}

// Open returns a Reader for the object stored under id, which is size bytes
// long. Telegram is not asked for anything until the first Read.
func (s *Store) Open(ctx context.Context, key []byte, id string, size int64) *Reader {
	return &Reader{store: s, ctx: ctx, key: key, id: id, size: size}
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.data == nil {
		if err := r.start(); err != nil {
			return 0, err
		}
	}

	index := int(r.offset / r.partSize)
	if index != r.index {
		if index >= len(r.parts) {
			return 0, io.ErrUnexpectedEOF
		}
		data, err := r.store.fetchPart(r.ctx, r.aead, r.parts[index].FileID, nil)
		if err != nil {
			return 0, err
		}
		r.index, r.data = index, data
	}

	start := r.offset - int64(index)*r.partSize
	if start >= int64(len(r.data)) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[start:])
	r.offset += int64(n)
	return n, nil
}

// start fetches the list of parts and the first part. Every part but the last
// is as long as the first, and that length says which part a position is in.
func (r *Reader) start() error {
	aead, err := newCipher(r.key)
	if err != nil {
		return err
	}
	m, _, err := r.store.fetchManifest(r.ctx, aead, r.id)
	if err != nil {
		return err
	}
	if len(m.Parts) == 0 {
		return io.ErrUnexpectedEOF
	}
	data, err := r.store.fetchPart(r.ctx, aead, m.Parts[0].FileID, nil)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return io.ErrUnexpectedEOF
	}
	r.aead, r.parts, r.partSize, r.data = aead, m.Parts, int64(len(data)), data
	return nil
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
