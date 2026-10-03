package tgstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeTelegram struct {
	mu        sync.Mutex
	files     map[string][]byte
	sent      int
	failSends int
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.URL.Path {
	case "/botTOKEN/sendDocument":
		if f.failSends > 0 {
			f.failSends--
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		file, _, err := r.FormFile("document")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(file)
		f.sent++
		id := fmt.Sprintf("file%d", f.sent)
		f.files[id] = data
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"document":{"file_id":%q}}}`, f.sent, id)
	case "/botTOKEN/deleteMessage":
		id := "file" + r.FormValue("message_id")
		if _, found := f.files[id]; !found {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "message to delete not found"})
			return
		}
		delete(f.files, id)
		fmt.Fprint(w, `{"ok":true,"result":true}`)
	case "/botTOKEN/getFile":
		id := r.FormValue("file_id")
		if _, found := f.files[id]; !found {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "file not found"})
			return
		}
		fmt.Fprintf(w, `{"ok":true,"result":{"file_path":%q}}`, "documents/"+id)
	default:
		var id string
		fmt.Sscanf(r.URL.Path, "/file/botTOKEN/documents/%s", &id)
		w.Write(f.files[id])
	}
}

func newTestStore(t *testing.T, partSize int) (*Store, *fakeTelegram) {
	fake := &fakeTelegram{files: map[string][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	store := New("TOKEN", 1)
	store.Endpoint = server.URL
	store.partSize = partSize
	return store, fake
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func TestRoundTrip(t *testing.T) {
	sizes := map[string]int{"empty": 0, "one part": 100, "exact parts": 4096, "many parts": 10*1024 + 123}
	for name, size := range sizes {
		t.Run(name, func(t *testing.T) {
			store, fake := newTestStore(t, 1024)
			key, content := randomBytes(32), randomBytes(size)

			id, err := store.Upload(context.Background(), key, bytes.NewReader(content))
			if err != nil {
				t.Fatal(err)
			}
			wantFiles := (size+1023)/1024 + 1
			fake.mu.Lock()
			if len(fake.files) != wantFiles {
				t.Fatalf("stored %d files, want %d", len(fake.files), wantFiles)
			}
			for _, stored := range fake.files {
				if size > 0 && bytes.Contains(stored, content[:min(size, 32)]) {
					t.Fatal("content was stored unencrypted")
				}
			}
			fake.mu.Unlock()

			var out bytes.Buffer
			if err := store.Download(context.Background(), key, id, &out); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), content) {
				t.Fatal("downloaded content differs from uploaded content")
			}
		})
	}
}

func TestWrongKey(t *testing.T) {
	store, _ := newTestStore(t, 1024)
	id, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(2000)))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Download(context.Background(), randomBytes(32), id, io.Discard); err == nil {
		t.Fatal("download with the wrong key succeeded")
	}
}

func TestRetry(t *testing.T) {
	store, fake := newTestStore(t, 1024)
	fake.failSends = 1
	key, content := randomBytes(32), randomBytes(500)

	id, err := store.Upload(context.Background(), key, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := store.Download(context.Background(), key, id, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), content) {
		t.Fatal("downloaded content differs from uploaded content")
	}
}

func TestUploadFailure(t *testing.T) {
	store, _ := newTestStore(t, 1024)
	store.Client = &http.Client{Transport: failFast{}}

	_, err := store.Upload(context.Background(), randomBytes(32), bytes.NewReader(randomBytes(5000)))
	if err == nil {
		t.Fatal("upload succeeded although every request failed")
	}
}

type failFast struct{}

func (failFast) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network is down")
}

func TestMissingObject(t *testing.T) {
	store, _ := newTestStore(t, 1024)
	for _, id := range []string{"nope", "7:nope"} {
		if err := store.Download(context.Background(), randomBytes(32), id, io.Discard); err == nil {
			t.Fatalf("download of missing object %q succeeded", id)
		}
	}
}

func TestDelete(t *testing.T) {
	store, fake := newTestStore(t, 1024)
	key := randomBytes(32)

	kept, err := store.Upload(context.Background(), key, bytes.NewReader(randomBytes(100)))
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Upload(context.Background(), key, bytes.NewReader(randomBytes(3000)))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), key, id); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	remaining := len(fake.files)
	fake.mu.Unlock()
	if remaining != 2 {
		t.Fatalf("%d files remain, want the 2 of the other object", remaining)
	}
	if err := store.Download(context.Background(), key, id, io.Discard); err == nil {
		t.Fatal("download of a deleted object succeeded")
	}
	if err := store.Download(context.Background(), key, kept, io.Discard); err != nil {
		t.Fatal(err)
	}
}
