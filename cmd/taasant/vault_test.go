package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedantlavale/taasant/s3store"
)

// telegramOnly is a vault with one backend, the way a setup from before there
// were several places looks.
func telegramOnly(b backend) *vault {
	return &vault{key: make([]byte, 32), def: "telegram", backends: map[string]backend{"telegram": b}}
}

func TestVaultGet(t *testing.T) {
	v := newVault(config{
		Key:     strings.Repeat("ab", 32),
		Default: "r2",
		Backends: map[string]target{
			"telegram": {Type: "telegram", Token: "TOKEN", ChatID: -100},
			"r2":       {Type: "telegram", Token: "OTHER", ChatID: -200},
			"broken":   {Type: "telegram"},
			"minio":    {Type: "s3", Endpoint: "http://localhost:9000", Region: "us-east-1", Bucket: "b", AccessKey: "a", SecretKey: "s"},
		},
	})
	def, err := v.get("")
	if err != nil {
		t.Fatal(err)
	}
	if named, _ := v.get("r2"); named != def {
		t.Error(`"" is not the default backend`)
	}
	if other, _ := v.get("telegram"); other == def {
		t.Error("two names gave the same backend")
	}
	_, err = v.get("broken")
	if err == nil || !strings.Contains(err.Error(), "minio, r2, telegram") {
		t.Errorf("an incomplete backend gave %v, want the places that work", err)
	}
	if _, ok := v.backends["minio"].(*s3store.Store); !ok {
		t.Error("an s3 target did not open as an s3 store")
	}
	if _, err := v.get("nowhere"); err == nil {
		t.Error("a name that is not set up gave a backend")
	}
}

func TestEntryWhere(t *testing.T) {
	if got := (entry{}).where(); got != "telegram" {
		t.Errorf("an entry from before backends is in %q, want telegram", got)
	}
	if got := (entry{Backend: "r2"}).where(); got != "r2" {
		t.Errorf("where is %q, want r2", got)
	}
}

func TestAddFileChoosesBackend(t *testing.T) {
	td := newTestDrive(t)
	file := filepath.Join(t.TempDir(), "note.txt")
	os.WriteFile(file, []byte("hello"), 0o644)
	report := func(done, total int64) {}

	added, err := addFile(context.Background(), td.d.vault, "", td.d.indexFile, file, report)
	if err != nil || added.Backend != "telegram" {
		t.Fatalf("an upload to the default went to %q, %v", added.Backend, err)
	}
	before := len(td.fake.files)
	if _, err := addFile(context.Background(), td.d.vault, "nowhere", td.d.indexFile, file, report); err == nil {
		t.Error("an upload to a place that is not set up worked")
	}
	if len(td.fake.files) != before {
		t.Error("something was uploaded although the place is not set up")
	}
}

func TestToFlag(t *testing.T) {
	for _, args := range [][]string{
		{"upload", "--to"},
		{"download", "--to", "r2", "name"},
	} {
		if err := run(args); err != errUsage {
			t.Errorf("run(%q) = %v, want the usage", args, err)
		}
	}
}

// Files in two places at once: each one is found where it was put.
func TestTwoPlaces(t *testing.T) {
	td := newTestDrive(t)
	s3 := fakeBucket(t)
	td.d.vault.backends["minio"] = s3store.New(s3.URL, "us-east-1", "files", "GOOD", "SECRET")
	v, ctx, report := td.d.vault, context.Background(), func(done, total int64) {}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("in telegram"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("in a bucket"), 0o644)
	a, err := addFile(ctx, v, "", td.d.indexFile, filepath.Join(dir, "a.txt"), report)
	if err != nil {
		t.Fatal(err)
	}
	b, err := addFile(ctx, v, "minio", td.d.indexFile, filepath.Join(dir, "b.txt"), report)
	if err != nil {
		t.Fatal(err)
	}
	if a.Backend != "telegram" || b.Backend != "minio" || len(s3.objects) != 1 {
		t.Fatalf("files went to %q and %q, %d objects in the bucket", a.Backend, b.Backend, len(s3.objects))
	}

	for e, want := range map[entry]string{a: "in telegram", b: "in a bucket"} {
		out := filepath.Join(dir, "out-"+e.Name)
		if err := download(ctx, v, e, out, report); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(out); string(got) != want {
			t.Errorf("%s came back as %q", e.Name, got)
		}
	}

	if err := removeFile(ctx, v, b, td.d.indexFile); err != nil || len(s3.objects) != 0 {
		t.Errorf("removing from the bucket: %v, %d objects left", err, len(s3.objects))
	}
	if entries, _ := loadIndex(td.d.indexFile); len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Errorf("the index has %+v", entries)
	}
}
