//go:build !windows

package main

import (
	"os"
	"syscall"
)

func lockFile(f *os.File) (func(), error) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, nil
}
