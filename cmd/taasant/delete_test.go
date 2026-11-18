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
