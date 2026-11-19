package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vedantlavale/taasant/s3store"
)

// Uploads and deletes to two places at once, all changing one index. Every
// file that was added and not deleted must be in the index once, and the
// index must still be readable.
func TestManyChangesToOneIndex(t *testing.T) {
	td := newTestDrive(t)
	s3 := fakeBucket(t)
	v := td.d.vault
	v.backends["minio"], v.kinds["minio"] = s3store.New(s3.URL, "us-east-1", "files", "GOOD", "SECRET"), "s3"
	ctx, report := context.Background(), func(done, total int64) {}

	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("content of "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Ten files to delete while the others are being added.
	var doomed []entry
	for i := range 10 {
		e, err := addFile(ctx, v, []string{"", "minio"}[i%2], td.d.indexFile, write(fmt.Sprintf("old%d", i)), report)
		if err != nil {
			t.Fatal(err)
		}
		doomed = append(doomed, e)
	}

	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			if _, err := addFile(ctx, v, []string{"", "minio"}[i%2], td.d.indexFile, write(fmt.Sprintf("new%d", i)), report); err != nil {
				t.Error(err)
			}
		})
	}
	for _, e := range doomed {
		wg.Go(func() {
			if _, err := removeFile(ctx, v, e, td.d.indexFile, nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	entries, err := loadIndex(td.d.indexFile)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, e := range entries {
		seen[e.Name]++
	}
	for i := range 40 {
		if name := fmt.Sprintf("new%d", i); seen[name] != 1 {
			t.Errorf("%s is in the index %d times", name, seen[name])
		}
	}
	for i := range 10 {
		if name := fmt.Sprintf("old%d", i); seen[name] != 0 {
			t.Errorf("%s was deleted and is still in the index", name)
		}
	}
	if len(entries) != 40 {
		t.Errorf("the index has %d files, want 40", len(entries))
	}
	// 5 of the old files and 20 of the new ones went to the bucket, minus the 5 deleted.
	if want := 20; len(s3.objects) != want {
		t.Errorf("the bucket holds %d objects, want %d", len(s3.objects), want)
	}
}

// The commands as someone types them, against a bucket: settings file in,
// index file out, nothing mocked between run and the store.
func TestCommandsAgainstABucket(t *testing.T) {
	s3 := fakeBucket(t)
	dir := t.TempDir()
	t.Setenv("TAAS_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("TAAS_INDEX", filepath.Join(dir, "index.json"))
	for _, name := range []string{"TG_BOT_TOKEN", "TG_CHAT_ID", "TAAS_KEY"} {
		t.Setenv(name, "")
	}
	conf := config{
		Key: "abababababababababababababababababababababababababababababababab", Default: "minio",
		Backends: map[string]target{"minio": {Type: "s3", Endpoint: s3.URL, Region: "us-east-1", Bucket: "files", AccessKey: "GOOD", SecretKey: "SECRET"}},
	}
	if err := saveConfig(os.Getenv("TAAS_CONFIG"), conf); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "holiday.jpg")
	os.WriteFile(file, []byte("the picture"), 0o644)

	if err := run([]string{"upload", file}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"upload", "--to", "minio", "--json", file}); err != nil {
		t.Fatal(err)
	}
	entries, _ := loadIndex(os.Getenv("TAAS_INDEX"))
	if len(entries) != 2 || entries[0].Backend != "minio" || len(s3.objects) != 3 { // two files and the one backup of the index, not one per change
		t.Fatalf("index %+v, %d objects", entries, len(s3.objects))
	}
	if err := run([]string{"upload", "--to", "elsewhere", file}); err == nil {
		t.Error("an upload to a place that is not set up worked")
	}
	if err := run([]string{"list"}); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "copy.jpg")
	if err := run([]string{"download", entries[0].ID, out}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "the picture" {
		t.Errorf("the download is %q", got)
	}
	if err := run([]string{"download", entries[0].ID, out}); err == nil {
		t.Error("a download overwrote a file that was there")
	}

	for _, e := range entries {
		if err := run([]string{"delete", e.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if left, _ := loadIndex(os.Getenv("TAAS_INDEX")); len(left) != 0 {
		t.Errorf("the index still has %+v", left)
	}
}
