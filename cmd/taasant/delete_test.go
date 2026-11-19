package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedantlavale/taasant/tgstore"
)

// Telegram will not delete messages older than 48 hours. The file must still
// come off the drive, with a note about the messages left in the chat.
func TestRemoveFileWithMessagesLeftBehind(t *testing.T) {
	fake := &fakeTelegram{files: map[string][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	store := tgstore.New("TOKEN", 1)
	store.Endpoint = server.URL

	dir := t.TempDir()
	file := filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(file, []byte("picture"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexFile := filepath.Join(dir, "index.json")
	v := telegramOnly(store)
	e, err := addFile(context.Background(), v, "", indexFile, file, func(done, total int64) {})
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	fake.refuseDelete = true
	fake.mu.Unlock()

	note, err := removeFile(context.Background(), v, e, indexFile, nil)
	if err != nil {
		t.Fatalf("deleting the file failed: %v", err)
	}
	if !strings.Contains(note, "still in the chat") || !strings.Contains(note, "screenshot.png") {
		t.Fatalf("the note is %q", note)
	}
	if !strings.Contains(note, "last 48 hours") {
		t.Fatalf("the note does not explain the 48-hour rule: %q", note)
	}
	entries, err := loadIndex(indexFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the file is still in the index: %v", entries)
	}
}

// A delete that failed for any reason but the age of the messages says nothing
// about the file, so the file must stay listed and the failure be reported.
func TestFailedTelegramDeleteKeepsTheFile(t *testing.T) {
	fake := &fakeTelegram{files: map[string][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	store := tgstore.New("TOKEN", 1)
	store.Endpoint = server.URL
	v := telegramOnly(store)

	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	os.WriteFile(file, []byte("hello"), 0o644)
	indexFile := filepath.Join(dir, "index.json")
	e, err := addFile(context.Background(), v, "", indexFile, file, func(done, total int64) {})
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	fake.brokenDelete = true
	fake.mu.Unlock()
	if _, err := removeFile(context.Background(), v, e, indexFile, nil); err == nil {
		t.Fatal("a delete that failed was reported as done")
	}
	if entries, _ := loadIndex(indexFile); len(entries) != 1 {
		t.Fatalf("the index has %d files, want the one that could not be deleted", len(entries))
	}
}

func TestPlaced(t *testing.T) {
	for _, test := range []struct {
		in      entry
		to      string
		backend string
		fails   bool
	}{
		{entry{ID: "1:abc", Name: "a"}, "", "", false},
		{entry{ID: "1:abc"}, "", "", false},
		{entry{ID: "0123456789abcdef0123456789abcdef"}, "", "", true},
		{entry{ID: "0123456789abcdef0123456789abcdef"}, "r2", "r2", false},
		{entry{ID: "0123456789abcdef0123456789abcdef", Name: "a", Backend: "r2"}, "", "r2", false},
	} {
		got, err := placed(test.in, test.to)
		if (err != nil) != test.fails || got.Backend != test.backend {
			t.Errorf("placed(%+v, %q) = %q, %v", test.in, test.to, got.Backend, err)
		}
	}
}
