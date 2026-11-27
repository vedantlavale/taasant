package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// bucket is an S3 server that accepts the access key GOOD and nothing else.
type bucket struct {
	*httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
}

func fakeBucket(t *testing.T) *bucket {
	b := &bucket{objects: map[string][]byte{}}
	b.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=GOOD/") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "<Error><Code>AccessDenied</Code><Message>no</Message></Error>")
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
			fmt.Fprint(w, "<InitiateMultipartUploadResult><UploadId>u1</UploadId></InitiateMultipartUploadResult>")
		case r.Method == http.MethodPut:
			b.objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case r.Method == http.MethodHead:
			if _, found := b.objects[r.URL.Path]; !found {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == http.MethodGet:
			w.Write(b.objects[r.URL.Path])
		case r.Method == http.MethodDelete:
			delete(b.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(b.Close)
	return b
}

// minio is the row of MinIO on the first screen.
var minio = []string{"down", "down", "down", "enter"}

func TestOnboardMinIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	server := fakeBucket(t).URL
	o := onboard{path: path, heard: -1}

	o = answer(o, minio...)
	if view := o.View(); o.step != askField || !strings.Contains(view, "step 1 of 4") || !strings.Contains(view, "Server address") {
		t.Fatalf("MinIO does not begin with the address:\n%s", view)
	}
	o = answer(o, "ftp://x", "enter")
	if o.step != askField || !strings.Contains(o.View(), "must start with http") {
		t.Fatalf("an address that is not one was accepted:\n%s", o.View())
	}
	o = answer(o, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", server, "enter")
	o = answer(o, "files", "enter", "GOOD", "enter")
	if view := o.View(); !strings.Contains(view, "Secret Key") || !strings.Contains(view, "step 4 of 4") {
		t.Fatalf("the secret was not asked for:\n%s", view)
	}
	o = answer(o, "hunter2", "enter")
	if o.step != ready {
		t.Fatalf("not finished:\n%s", o.View())
	}

	saved, err := loadConfig(path)
	want := target{Type: "s3", Endpoint: server, Region: "us-east-1", Bucket: "files", AccessKey: "GOOD", SecretKey: "hunter2"}
	if err != nil || saved.Backends["minio"] != want || saved.Default != "minio" || saved.check() != nil {
		t.Errorf("saved %+v, %v", saved, err)
	}
	if !strings.Contains(o.View(), "New files go to minio") {
		t.Errorf("the last screen does not say where files go:\n%s", o.View())
	}
}

func TestOnboardBadKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	server := fakeBucket(t).URL
	o := onboard{path: path, heard: -1}

	o = answer(o, minio...)
	o = answer(o, server, "enter", "files", "enter", "BAD", "enter", "hunter2")
	if strings.Contains(o.View(), "hunter2") {
		t.Fatalf("the secret is on the screen:\n%s", o.View())
	}
	o = answer(o, "enter")
	if view := o.View(); o.step != checkField || !strings.Contains(view, "AccessDenied") {
		t.Fatalf("bad keys were not reported:\n%s", view)
	}
	if _, err := loadConfig(path); err != nil || o.step == ready {
		t.Fatal("something was saved although the bucket does not work")
	}

	// Back to the secret, and back once more to the access key.
	o = answer(o, "esc", "esc")
	if o.step != askField || !strings.Contains(o.View(), "Access Key") || o.input != "BAD" {
		t.Fatalf("Esc did not go back to the access key with its answer:\n%s", o.View())
	}
	o = answer(o, "backspace", "backspace", "backspace", "GOOD", "enter", "hunter2", "enter")
	if o.step != ready {
		t.Fatalf("not finished after fixing the key:\n%s", o.View())
	}
}

func TestOnboardEscFromFirstQuestion(t *testing.T) {
	o := answer(onboard{heard: -1}, minio...)
	o = answer(o, "esc")
	if o.step != askBackend || o.choice != 3 {
		t.Errorf("Esc went to step %d, choice %d, want the first screen on MinIO", o.step, o.choice)
	}
}

// Adding a place to settings that work must not take the default away unless
// the person says so.
func TestOnboardAddsAPlace(t *testing.T) {
	key := strings.Repeat("ab", 32)
	for _, test := range []struct {
		keys    []string
		want    string
		message string
	}{
		{[]string{"enter"}, "minio", "yes"},
		{[]string{"down", "enter"}, "telegram", "no"},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		server := fakeBucket(t).URL
		o := onboard{path: path, heard: -1, again: true, conf: telegramConfig("TOKEN", -100, key)}

		o = answer(o, minio...)
		o = answer(o, server, "enter", "files", "enter", "GOOD", "enter", "hunter2", "enter")
		if view := o.View(); o.step != askDefault || !strings.Contains(view, "MinIO or other S3 works") {
			t.Fatalf("%s: the default was not asked about:\n%s", test.message, view)
		}
		o = answer(o, test.keys...)
		saved, err := loadConfig(path)
		if o.step != ready || err != nil || saved.Default != test.want || len(saved.Backends) != 2 || saved.Key != key {
			t.Errorf("%s: step %d, saved %+v, %v", test.message, o.step, saved, err)
		}
		if view := o.View(); !strings.Contains(view, "--to") || !strings.Contains(view, "Enter Done") {
			t.Errorf("%s: the last screen does not explain --to:\n%s", test.message, view)
		}
	}
}

func TestProviderTargets(t *testing.T) {
	byName := map[string]provider{}
	for _, p := range providers {
		byName[p.name] = p
	}
	got := byName["r2"].build([]string{"0123456789abcdef0123456789abcdef", "files", "A", "S"})
	want := target{Type: "s3", Endpoint: "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com", Region: "auto", Bucket: "files", AccessKey: "A", SecretKey: "S"}
	if got != want || got.check() != nil {
		t.Errorf("r2 builds %+v", got)
	}
	got = byName["s3"].build([]string{"eu-west-1", "files", "A", "S"})
	want = target{Type: "s3", Endpoint: "https://s3.eu-west-1.amazonaws.com", Region: "eu-west-1", Bucket: "files", AccessKey: "A", SecretKey: "S"}
	if got != want || got.check() != nil {
		t.Errorf("s3 builds %+v", got)
	}

	if isAccountID("0123456789abcdef0123456789abcdef") != "" {
		t.Error("a real account ID was refused")
	}
	for _, wrong := range []string{"files", "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com"} {
		if isAccountID(wrong) == "" {
			t.Errorf("%q was taken for an account ID", wrong)
		}
	}
}

// Setting up a place again fixes its keys. A different bucket is another
// place, and must not take the name of the first: files are stored in it.
func TestOnboardKeepsAPlaceThatHasOtherFiles(t *testing.T) {
	key := strings.Repeat("ab", 32)
	first, second := fakeBucket(t), fakeBucket(t)

	setup := func(conf config, server *bucket, bucket string) config {
		path := filepath.Join(t.TempDir(), "config.json")
		o := onboard{path: path, heard: -1, again: true, conf: conf}
		o = answer(o, minio...)
		o = answer(o, server.URL, "enter", bucket, "enter", "GOOD", "enter", "hunter2", "enter")
		if o.step == askDefault {
			o = answer(o, "down", "enter")
		}
		saved, err := loadConfig(path)
		if o.step != ready || err != nil {
			t.Fatalf("step %d: %v\n%s", o.step, err, o.View())
		}
		return saved
	}

	conf := setup(telegramConfig("TOKEN", -100, key), first, "photos")
	if _, found := conf.Backends["minio"]; !found {
		t.Fatalf("the first place was not saved as minio: %+v", conf.Backends)
	}
	same := setup(conf, first, "photos")
	if len(same.Backends) != 2 {
		t.Errorf("the same bucket again made a third place: %+v", same.Backends)
	}
	other := setup(conf, second, "videos")
	if len(other.Backends) != 3 || other.Backends["minio"].Bucket != "photos" || other.Backends["minio-2"].Bucket != "videos" {
		t.Errorf("another bucket replaced the first one: %+v", other.Backends)
	}
}

func TestBucketNames(t *testing.T) {
	for _, good := range []string{"files", "my-files.2026", "abc", "0123456789"} {
		if isBucket(good) != "" {
			t.Errorf("%q was refused", good)
		}
	}
	for _, bad := range []string{"", "ab", "Files", "my files", "a/b", "files?x=1", "-files", "files-", strings.Repeat("a", 64)} {
		if isBucket(bad) == "" {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestUploadRefusesWhatItCannotSize(t *testing.T) {
	td := newTestDrive(t)
	if _, err := addFile(context.Background(), td.d.vault, "", td.d.indexFile, "/dev/null", func(done, total int64) {}); err == nil {
		t.Error("a device was uploaded as a file")
	}
}
