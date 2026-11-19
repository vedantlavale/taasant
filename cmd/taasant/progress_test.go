package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransfers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json.progress")
	tr := transfers{path: path}
	read := func() progressFile {
		var got progressFile
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(data, &got)
		return got
	}

	tr.add(500) // the backup of the index, before any file
	a, b := &writing{}, &writing{}
	tr.begin(a, "a.dmg", 1000)
	if got := read(); got.Done != 0 || got.Total != 1000 || len(got.Files) != 1 {
		t.Fatalf("after the first file: %+v", got)
	}

	tr.begin(b, "b.png", 500)
	time.Sleep(600 * time.Millisecond) // writes are at most twice a second
	tr.add(300)
	if got := read(); got.Done != 300 || got.Total != 1500 || got.Files[0] != "a.dmg" || got.Files[1] != "b.png" {
		t.Fatalf("with two files: %+v", got)
	}

	tr.end(a)
	if got := read(); len(got.Files) != 1 || got.Files[0] != "b.png" {
		t.Fatalf("after one ended: %+v", got)
	}
	tr.end(b)
	if _, err := os.Stat(path); err == nil {
		t.Error("the file stays after the last transfer")
	}

	// Without a path nothing is written.
	none := transfers{}
	none.begin(a, "a", 1)
	none.add(1)
	none.end(a)
}
