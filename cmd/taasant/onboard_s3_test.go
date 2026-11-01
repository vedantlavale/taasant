package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBucket is an S3 server that accepts the access key GOOD and nothing else.
func fakeBucket(t *testing.T) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=GOOD/") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "<Error><Code>AccessDenied</Code><Message>no</Message></Error>")
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// minio is the row of MinIO on the first screen.
var minio = []string{"down", "down", "down", "enter"}

func TestOnboardMinIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	server := fakeBucket(t)
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
	server := fakeBucket(t)
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
		server := fakeBucket(t)
		o := onboard{path: path, heard: -1, again: true, conf: telegramConfig("TOKEN", -100, key)}

		o = answer(o, minio...)
		o = answer(o, server, "enter", "files", "enter", "GOOD", "enter", "hunter2", "enter")
		if view := o.View(); o.step != askDefault || !strings.Contains(view, "MinIO works") {
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
