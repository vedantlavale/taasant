package tgstore

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// dropFirst cuts the connection on the first few requests, the way a bad
// network does, and then lets the fake Telegram answer.
func dropFirst(t *testing.T, drops int) (*Store, *fakeTelegram) {
	retryWait = time.Millisecond
	t.Cleanup(func() { retryWait = time.Second })

	fake := &fakeTelegram{files: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if drops > 0 {
			drops--
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	store := New("TOKEN", 1)
	store.Endpoint = server.URL
	store.Workers = 1
	return store, fake
}

func TestBrokenConnectionIsRetried(t *testing.T) {
	store, _ := dropFirst(t, 2)
	key, content := randomBytes(32), randomBytes(3000)

	reported := 0
	store.Progress = func(bytes int) { reported += bytes }
	id, err := store.Upload(context.Background(), key, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if reported != len(content) {
		t.Errorf("progress after two failed attempts adds up to %d, want %d", reported, len(content))
	}

	var got bytes.Buffer
	if err := store.Download(context.Background(), key, id, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), content) {
		t.Error("downloaded something else than was uploaded")
	}
}

// A network error names the address it failed on, and that address holds
// the bot token.
func TestErrorHidesToken(t *testing.T) {
	store, _ := dropFirst(t, 100)
	_, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(10)))
	if err == nil {
		t.Fatal("upload over a dead connection succeeded")
	}
	if strings.Contains(err.Error(), "TOKEN") {
		t.Errorf("the error shows the token: %v", err)
	}
}

func TestProgressMovesDuringAPart(t *testing.T) {
	store, _ := newTestStore(t, 1<<20)
	calls := 0
	store.Progress = func(bytes int) { calls++ }
	if _, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(1<<20))); err != nil {
		t.Fatal(err)
	}
	if calls < 5 {
		t.Errorf("one part of 1 MiB reported progress %d times, want it to move while sending", calls)
	}
}
