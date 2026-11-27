package s3store

import (
	"bytes"
	"context"
	"io"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
	"testing/iotest"
)

// Whatever the sizes and however the reader hands over its bytes, what goes
// in is what comes out, through a download and through seeks and reads.
func TestRandomSizesAndReads(t *testing.T) {
	store, _ := newFake(t)
	rng := rand.New(rand.NewPCG(1, 2))
	key := randomBytes(32)
	wrap := []func(io.Reader) io.Reader{
		func(r io.Reader) io.Reader { return r },
		iotest.OneByteReader,
		iotest.HalfReader,
		iotest.DataErrReader,
	}

	for i := range 150 {
		store.chunkSize = 16 + rng.IntN(300)
		size := rng.IntN(5*store.chunkSize + 4)
		content := randomBytes(size)
		id, err := store.Upload(context.Background(), key, wrap[i%len(wrap)](bytes.NewReader(content)), nil)
		if err != nil {
			t.Fatalf("chunk %d, size %d: %v", store.chunkSize, size, err)
		}
		var got bytes.Buffer
		if err := store.Download(context.Background(), key, id, &got, nil); err != nil || !bytes.Equal(got.Bytes(), content) {
			t.Fatalf("chunk %d, size %d: download gave other bytes, or %v", store.chunkSize, size, err)
		}

		r := store.Open(context.Background(), key, id, int64(size))
		for range 6 {
			if size == 0 {
				break
			}
			offset := rng.IntN(size)
			length := 1 + rng.IntN(3*store.chunkSize)
			r.Seek(int64(offset), io.SeekStart)
			buf := make([]byte, length)
			n, err := io.ReadFull(r, buf)
			if err != nil && err != io.ErrUnexpectedEOF {
				t.Fatalf("chunk %d, size %d, read at %d: %v", store.chunkSize, size, offset, err)
			}
			if !bytes.Equal(buf[:n], content[offset:min(offset+length, size)]) {
				t.Fatalf("chunk %d, size %d: %d bytes at %d are other bytes", store.chunkSize, size, length, offset)
			}
		}
		store.chunks, store.order = nil, nil
	}
}

// Many uploads, downloads and readers at once on one store, for the race detector.
func TestManyAtOnce(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Go(func() {
			content := randomBytes(1 + (i*977)%(6*testChunk))
			id, err := store.Upload(context.Background(), key, bytes.NewReader(content), func(int) {})
			if err != nil {
				t.Error(err)
				return
			}
			var got bytes.Buffer
			if err := store.Download(context.Background(), key, id, &got, func(int) {}); err != nil || !bytes.Equal(got.Bytes(), content) {
				t.Errorf("upload %d came back as other bytes, or %v", i, err)
			}
			if again, err := io.ReadAll(store.Open(context.Background(), key, id, int64(len(content)))); err != nil || !bytes.Equal(again, content) {
				t.Errorf("reader %d got other bytes, or %v", i, err)
			}
			if err := store.Delete(context.Background(), key, id, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(fake.objects) != 0 || len(fake.uploads) != 0 {
		t.Errorf("%d objects and %d uploads left", len(fake.objects), len(fake.uploads))
	}
}

// One part more than S3 allows is refused, and nothing is left in the bucket.
func TestTooManyParts(t *testing.T) {
	store, fake := newFake(t)
	store.chunkSize = 1
	store.Workers = 8
	_, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(maxParts+1)), nil)
	if err == nil {
		t.Fatal("an upload of more than the most parts worked")
	}
	if len(fake.uploads) != 0 || len(fake.objects) != 0 {
		t.Errorf("%d uploads and %d objects were left", len(fake.uploads), len(fake.objects))
	}
}

// A download cut short in the middle of the body must fail, not return what
// it had as if it were the file.
func TestDownloadCutOffByTheNetwork(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	id, err := store.Upload(context.Background(), key, bytes.NewReader(randomBytes(3*testChunk)), nil)
	if err != nil {
		t.Fatal(err)
	}
	full := fake.objects[id]
	for _, cut := range []int{1, 27, 28, testChunk, testChunk + overhead, 2*testChunk + 5, len(full) - 1} {
		fake.objects[id] = full[:cut]
		if err := store.Download(context.Background(), key, id, io.Discard, nil); err == nil {
			t.Errorf("an object cut at %d bytes was downloaded without an error", cut)
		}
	}
}

func TestCorruptionIsAlwaysFound(t *testing.T) {
	store, fake := newFake(t)
	key := randomBytes(32)
	content := randomBytes(2*testChunk + 77)
	id, _ := store.Upload(context.Background(), key, bytes.NewReader(content), nil)
	good := bytes.Clone(fake.objects[id])
	rng := rand.New(rand.NewPCG(3, 4))
	for i := range 400 {
		bad := bytes.Clone(good)
		switch rng.IntN(3) {
		case 0:
			bad[rng.IntN(len(bad))] ^= byte(1 + rng.IntN(255))
		case 1:
			bad = bad[:rng.IntN(len(bad))]
		case 2:
			at := rng.IntN(len(bad))
			bad = append(bad[:at], append(randomBytes(1+rng.IntN(40)), bad[at:]...)...)
		}
		fake.objects[id] = bad
		var got bytes.Buffer
		err := store.Download(context.Background(), key, id, &got, nil)
		if err == nil && !bytes.Equal(got.Bytes(), content) {
			t.Fatalf("mutation %d (%s) came back as other bytes without an error", i, strconv.Itoa(len(bad)))
		}
		if err == nil && !bytes.Equal(bad, good) {
			t.Fatalf("mutation %d changed the object and was still accepted", i)
		}
	}
}
