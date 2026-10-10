package tgstore

import (
	"bytes"
	"context"
	"io"
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
