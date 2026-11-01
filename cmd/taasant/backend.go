package main

import (
	"context"
	"io"
)

// backend is a place files can be stored. Every method takes the encryption
// key, and progress, if not nil, hears how many bytes have moved since its last call.
type backend interface {
	Upload(ctx context.Context, key []byte, r io.Reader, progress func(bytes int)) (id string, err error)
	Download(ctx context.Context, key []byte, id string, w io.Writer, progress func(bytes int)) error
	Delete(ctx context.Context, key []byte, id string) error
	Open(ctx context.Context, key []byte, id string, size int64) io.ReadSeeker
}
