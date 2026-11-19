package main

import (
	"os"
	"path/filepath"
)

// lockIndex holds the lock for an index file until unlock is called. The
// mounted drive and the commands can run at the same time, and both change
// the index; without this one lock, the later save would overwrite the
// earlier one and lose a file from the list.
func lockIndex(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	unlock, err := lockFile(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() { unlock(); f.Close() }, nil
}
