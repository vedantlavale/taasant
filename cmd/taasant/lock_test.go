package main

import (
	"path/filepath"
	"testing"
	"time"
)

// The drive and a command can run at once. Only one of them may change the
// index at a time.
func TestIndexLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	unlock, err := lockIndex(path)
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})
	go func() {
		other, err := lockIndex(path)
		if err != nil {
			return
		}
		close(acquired)
		other()
	}()

	select {
	case <-acquired:
		t.Fatal("a second writer got the lock while the first held it")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	select {
	case <-acquired:
	case <-time.After(3 * time.Second):
		t.Fatal("the second writer never got the lock")
	}
}
