package tgstore

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestProgress(t *testing.T) {
	store, _ := newTestStore(t, 1024)
	key, content := randomBytes(32), randomBytes(5000)

	reported := 0
	store.Progress = func(bytes int) { reported += bytes }

	id, err := store.Upload(context.Background(), key, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if reported != len(content) {
		t.Fatalf("upload reported %d bytes, want %d", reported, len(content))
	}

	reported = 0
	if err := store.Download(context.Background(), key, id, io.Discard); err != nil {
		t.Fatal(err)
	}
	if reported != len(content) {
		t.Fatalf("download reported %d bytes, want %d", reported, len(content))
	}
}
