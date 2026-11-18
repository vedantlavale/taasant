package tgstore

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
)

func TestReader(t *testing.T) {
	store, fake := newTestStore(t, 10)
	ctx := context.Background()
	key := randomBytes(32)
	data := randomBytes(35)

	id, err := store.Upload(ctx, key, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(store.Open(ctx, key, id, int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("reading from the start gave other bytes")
	}

	// A change of the part size must not matter for what is already stored.
	store.partSize = 4
	for _, from := range []int64{0, 9, 10, 27, 34} {
		r := store.Open(ctx, key, id, int64(len(data)))
		if _, err := r.Seek(from, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data[from:]) {
			t.Fatalf("reading from %d gave other bytes", from)
		}
	}

	r := store.Open(ctx, key, id, int64(len(data)))
	if at, err := r.Seek(-5, io.SeekEnd); err != nil || at != 30 {
		t.Fatalf("Seek from the end went to %d, %v", at, err)
	}
	if _, err := r.Seek(-1, io.SeekStart); err == nil {
		t.Fatal("Seek to before the start worked")
	}

	// A read near the end needs the first part and the last, not the two between.
	aead, _ := newCipher(key)
	m, _, err := store.fetchManifest(ctx, aead, id)
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	delete(fake.files, m.Parts[1].FileID)
	delete(fake.files, m.Parts[2].FileID)
	fake.mu.Unlock()
	last := make([]byte, 5)
	if _, err := io.ReadFull(r, last); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(last, data[30:]) {
		t.Fatal("reading the end gave other bytes")
	}
}

// Open and Seek ask Telegram for nothing, so they work even for an object
// that is not there. The first Read finds out.
// Readers of one object with different keys must not share decrypted bytes
// or the shape of the object through the store's caches.
func TestReaderCacheIsKeyedByKey(t *testing.T) {
	store, _ := newTestStore(t, 10)
	ctx := context.Background()
	keyA, keyB := randomBytes(32), randomBytes(32)
	data := randomBytes(35)
	id, err := store.Upload(ctx, keyA, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	first, err := io.ReadAll(store.Open(ctx, keyA, id, int64(len(data))))
	if err != nil || !bytes.Equal(first, data) {
		t.Fatalf("reading with the right key gave %v, %v", first, err)
	}
	second, err := io.ReadAll(store.Open(ctx, keyB, id, int64(len(data))))
	if err == nil && bytes.Equal(second, data) {
		t.Fatal("a reader with another key got the first key's plaintext")
	}
}

func TestReaderMissingObject(t *testing.T) {
	store, _ := newTestStore(t, 10)
	r := store.Open(context.Background(), randomBytes(32), "1:missing", 20)
	if _, err := r.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(make([]byte, 1)); err == nil {
		t.Fatal("reading an object that is not there worked")
	}
}

// Finder reads one file through several Readers at once. Together they must
// not download a part more than once.
func TestReadersShare(t *testing.T) {
	store, fake := newTestStore(t, 10)
	ctx := context.Background()
	key := randomBytes(32)
	data := randomBytes(35)

	id, err := store.Upload(ctx, key, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := io.ReadAll(store.Open(ctx, key, id, int64(len(data))))
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("a Reader got other bytes, or %v", err)
			}
		})
	}
	wg.Wait()

	// Each Reader may ask for the list of parts, but the 4 parts come once.
	fake.mu.Lock()
	downloads := fake.downloads
	fake.mu.Unlock()
	if downloads > 8+4 {
		t.Fatalf("8 Readers made %d downloads", downloads)
	}

	// Everything is still in memory, so a Reader later needs nothing.
	r := store.Open(ctx, key, id, int64(len(data)))
	r.Seek(12, io.SeekStart)
	got, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(got, data[12:]) {
		t.Fatalf("a later Reader got other bytes, or %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.downloads != downloads {
		t.Fatalf("a later Reader made %d more downloads", fake.downloads-downloads)
	}
}

// More parts than the cache can hold must not make Readers download the
// same part twice while it is still on its way.
func TestReadersDoNotRefetchPendingParts(t *testing.T) {
	store, fake := newTestStore(t, 10)
	ctx := context.Background()
	key := randomBytes(32)
	data := randomBytes(80) // 8 parts, more than keep
	id, err := store.Upload(ctx, key, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	// The first Read fills the shape cache. From here each Reader needs one
	// manifest and one part, at most.
	io.ReadAll(store.Open(ctx, key, id, int64(len(data))))
	fake.mu.Lock()
	before := fake.downloads
	fake.mu.Unlock()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := io.ReadAll(store.Open(ctx, key, id, int64(len(data))))
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("a Reader got other bytes, or %v", err)
			}
		})
	}
	wg.Wait()

	fake.mu.Lock()
	extra := fake.downloads - before
	fake.mu.Unlock()
	if extra > 16 { // 8 manifests and 8 parts
		t.Fatalf("8 Readers made %d downloads, some parts were fetched twice", extra)
	}
}
