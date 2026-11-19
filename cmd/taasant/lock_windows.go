//go:build windows

package main

import "os"

// The Finder drive only runs on a Mac. On Windows there is no second writer
// of the index to guard against, so the lock is a no-op there.
func lockFile(f *os.File) (func(), error) {
	return func() {}, nil
}
