package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// output returns what f prints to standard output.
func output(t *testing.T, f func()) string {
	t.Helper()
	read, write, _ := os.Pipe()
	saved := os.Stdout
	os.Stdout = write
	f()
	os.Stdout = saved
	write.Close()
	data, _ := io.ReadAll(read)
	return string(data)
}

func TestListJSON(t *testing.T) {
	index := filepath.Join(t.TempDir(), "index.json")
	t.Setenv("TAAS_INDEX", index)

	// No file yet must still be a list, which a program can read.
	out := output(t, func() { run([]string{"list", "--json"}) })
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("an empty list is %q, want []", out)
	}

	saveIndex(index, sample)
	var got []entry
	out = output(t, func() { run([]string{"list", "--json"}) })
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != len(sample) || got[0].ID != "1:a" {
		t.Errorf("the list is %q (%v)", out, err)
	}
}

func TestProgressJSON(t *testing.T) {
	out := output(t, func() {
		report := progressFor("a.bin", true)
		report(0, 1000)
		report(5, 1000)
		report(500, 1000)
		report(1000, 1000)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// 0 and 5 are both 0 percent, which is one line.
	if len(lines) != 3 || lines[2] != `{"done":1000,"total":1000}` {
		t.Errorf("the progress is %q", lines)
	}
}
