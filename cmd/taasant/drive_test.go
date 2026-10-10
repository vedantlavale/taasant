package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedantlavale/taasant/tgstore"
)

const testHost = "taasant.localhost:8737"

// testDrive is a drive on a fake Telegram, behind the server Finder talks to.
type testDrive struct {
	t      *testing.T
	d      *drive
	fake   *fakeTelegram
	server *httptest.Server
}

func newTestDrive(t *testing.T) *testDrive {
	fake := &fakeTelegram{files: map[string][]byte{}}
	telegram := httptest.NewServer(fake)
	t.Cleanup(telegram.Close)
	store := tgstore.New("TOKEN", 1)
	store.Endpoint = telegram.URL

	d := newDrive(store, make([]byte, 32), filepath.Join(t.TempDir(), "index.json"))
	server := httptest.NewServer(d.handler(testHost))
	t.Cleanup(server.Close)
	return &testDrive{t, d, fake, server}
}

// do sends one request the way Finder would and returns the status and the body.
func (td *testDrive) do(method, name, body string, header ...string) (int, string) {
	td.t.Helper()
	req, err := http.NewRequest(method, td.server.URL+"/taasant/"+name, strings.NewReader(body))
	if err != nil {
		td.t.Fatal(err)
	}
	req.Host = testHost
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		td.t.Fatal(err)
	}
	defer res.Body.Close()
	got, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(got)
}

// names returns the names in the index, in order.
func (td *testDrive) names() string {
	td.t.Helper()
	entries, err := loadIndex(td.d.indexFile)
	if err != nil {
		td.t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return strings.Join(names, " ")
}

// inChat counts the messages the fake Telegram holds.
func (td *testDrive) inChat() int {
	td.fake.mu.Lock()
	defer td.fake.mu.Unlock()
	return len(td.fake.files)
}

func TestDriveFiles(t *testing.T) {
	td := newTestDrive(t)

	if status, _ := td.do("PUT", "notes.txt", "hello"); status != http.StatusCreated {
		t.Fatalf("PUT answered %d", status)
	}
	if td.names() != "notes.txt" || td.inChat() != 2 {
		t.Fatalf("after one PUT the index has %q and the chat %d messages", td.names(), td.inChat())
	}
	if status, got := td.do("GET", "notes.txt", ""); status != http.StatusOK || got != "hello" {
		t.Fatalf("GET answered %d %q", status, got)
	}
	if status, got := td.do("GET", "notes.txt", "", "Range", "bytes=1-3"); status != http.StatusPartialContent || got != "ell" {
		t.Fatalf("GET of a range answered %d %q", status, got)
	}
	if _, got := td.do("PROPFIND", "", "", "Depth", "1"); !strings.Contains(got, "notes.txt") {
		t.Fatalf("the listing does not have the file: %s", got)
	}

	// Saving over a file deletes the old messages.
	td.do("PUT", "notes.txt", "goodbye")
	if td.names() != "notes.txt" || td.inChat() != 2 {
		t.Fatalf("after saving over it the index has %q and the chat %d messages", td.names(), td.inChat())
	}
	if _, got := td.do("GET", "notes.txt", ""); got != "goodbye" {
		t.Fatalf("GET after saving over it answered %q", got)
	}
}

func TestDriveFolders(t *testing.T) {
	td := newTestDrive(t)

	if status, _ := td.do("PUT", "text/notes.txt", "hello"); status != http.StatusConflict {
		t.Fatalf("PUT into a folder that is not there answered %d", status)
	}
	if status, _ := td.do("MKCOL", "text", ""); status != http.StatusCreated {
		t.Fatalf("MKCOL answered %d", status)
	}
	td.do("PUT", "text/notes.txt", "hello")
	td.do("PUT", "top.txt", "top")
	if td.names() != "text/notes.txt top.txt" {
		t.Fatalf("the index has %q", td.names())
	}
	if _, got := td.do("PROPFIND", "", "", "Depth", "1"); !strings.Contains(got, "/taasant/text/") || strings.Contains(got, "notes.txt") {
		t.Fatalf("the top listing should show the folder and not what is in it: %s", got)
	}

	// Moving a folder renames what is in it and uploads nothing.
	sent := td.fake.sent
	destination := "http://" + testHost + "/taasant/words"
	if status, _ := td.do("MOVE", "text", "", "Destination", destination); status != http.StatusCreated {
		t.Fatalf("MOVE answered %d", status)
	}
	if td.names() != "words/notes.txt top.txt" || td.fake.sent != sent {
		t.Fatalf("after the move the index has %q", td.names())
	}
	if _, got := td.do("GET", "words/notes.txt", ""); got != "hello" {
		t.Fatalf("GET after the move answered %q", got)
	}

	if status, _ := td.do("DELETE", "words", ""); status != http.StatusNoContent {
		t.Fatalf("DELETE answered %d", status)
	}
	if td.names() != "top.txt" || td.inChat() != 2 {
		t.Fatalf("after deleting the folder the index has %q and the chat %d messages", td.names(), td.inChat())
	}
	if status, _ := td.do("PROPFIND", "words", "", "Depth", "0"); status != http.StatusNotFound {
		t.Fatalf("the deleted folder answered %d", status)
	}
}

// Finder's helper files and empty files are kept by the drive and not sent.
func TestDriveLocalFiles(t *testing.T) {
	td := newTestDrive(t)

	td.do("PUT", ".DS_Store", "finder")
	td.do("PUT", "._notes.txt", "attributes")
	td.do("PUT", "empty.txt", "")
	if td.names() != "" || td.inChat() != 0 {
		t.Fatalf("the index has %q and the chat %d messages", td.names(), td.inChat())
	}
	if status, got := td.do("GET", ".DS_Store", ""); status != http.StatusOK || got != "finder" {
		t.Fatalf("GET of a helper file answered %d %q", status, got)
	}
	if status, got := td.do("GET", "empty.txt", ""); status != http.StatusOK || got != "" {
		t.Fatalf("GET of an empty file answered %d %q", status, got)
	}

	// Finder makes a file empty first and fills it with a second request.
	td.do("PUT", "empty.txt", "now with text")
	if td.names() != "empty.txt" {
		t.Fatalf("the index has %q", td.names())
	}
	if _, got := td.do("GET", "empty.txt", ""); got != "now with text" {
		t.Fatalf("GET answered %q", got)
	}
}

// A file uploaded by a command while the drive is mounted shows up on it.
func TestDriveSeesCommands(t *testing.T) {
	td := newTestDrive(t)
	td.do("PUT", "first.txt", "first")

	file := filepath.Join(t.TempDir(), "second.txt")
	os.WriteFile(file, []byte("second"), 0o644)
	_, err := addFile(context.Background(), td.d.store, td.d.key, td.d.indexFile, file, func(done, total int64) {})
	if err != nil {
		t.Fatal(err)
	}
	if status, got := td.do("GET", "second.txt", ""); status != http.StatusOK || got != "second" {
		t.Fatalf("GET answered %d %q", status, got)
	}
}

// An upload that breaks off must not be saved as if it were the whole file.
func TestDriveBrokenUpload(t *testing.T) {
	td := newTestDrive(t)

	reader, writer := io.Pipe()
	req, _ := http.NewRequest("PUT", td.server.URL+"/taasant/big.bin", reader)
	req.Host = testHost
	go func() {
		writer.Write([]byte("only the start"))
		writer.CloseWithError(errors.New("the connection broke"))
	}()
	if res, err := http.DefaultClient.Do(req); err == nil {
		res.Body.Close()
		t.Fatalf("the broken PUT answered %d", res.StatusCode)
	}

	// Close waits until the request has been dealt with.
	td.server.Close()
	if td.names() != "" {
		t.Fatalf("the index has %q", td.names())
	}
}

func TestDriveOtherHost(t *testing.T) {
	td := newTestDrive(t)
	td.do("PUT", "notes.txt", "hello")

	req, _ := http.NewRequest("GET", td.server.URL+"/taasant/notes.txt", nil)
	req.Host = "example.com"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("a request under another name answered %d", res.StatusCode)
	}
}
