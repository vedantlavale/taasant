package main

import (
	"encoding/json"
	"os"
	"slices"
	"sync"
	"time"
)

// transfers tells the Mac app how far the files that Finder is copying to the
// drive have got. Finder itself shows the copy as nearly done long before
// the upload is, because it only hands the bytes over.
//
// The numbers go into a small file that the app reads once a second. The
// file is there only while something is being saved. With no path it does
// nothing, as in the tests.
type transfers struct {
	path string

	mu      sync.Mutex
	active  map[*writing]string // the files being saved, by name
	total   int64               // what the files that began since the quiet will come to
	done    int64
	written time.Time // when the file was last written
}

// progressFile is what the app reads.
type progressFile struct {
	Files []string `json:"files"`
	Done  int64    `json:"done"`
	Total int64    `json:"total"` // 0 when Finder did not say how big a file is
}

func (t *transfers) begin(w *writing, name string, size int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.active) == 0 {
		// Whatever was counted before, such as the backup of the index, is not this.
		t.active, t.total, t.done = map[*writing]string{}, 0, 0
	}
	t.active[w] = name
	t.total += size
	t.write(true)
}

func (t *transfers) end(w *writing) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.active, w)
	if len(t.active) == 0 {
		t.total, t.done = 0, 0
		if t.path != "" {
			os.Remove(t.path)
		}
		return
	}
	t.write(true)
}

// add is called by the store as bytes reach Telegram.
func (t *transfers) add(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done += int64(n)
	t.write(false)
}

// write saves the numbers, at most twice a second unless it is forced.
func (t *transfers) write(force bool) {
	if t.path == "" || len(t.active) == 0 || (!force && time.Since(t.written) < 500*time.Millisecond) {
		return
	}
	t.written = time.Now()
	names := make([]string, 0, len(t.active))
	for _, name := range t.active {
		names = append(names, name)
	}
	slices.Sort(names)
	data, err := json.Marshal(progressFile{names, t.done, t.total})
	if err == nil {
		writeFile(t.path, data)
	}
}
