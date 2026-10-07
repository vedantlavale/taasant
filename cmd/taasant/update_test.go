package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
)

func TestInstall(t *testing.T) {
	target := filepath.Join(t.TempDir(), "taasant")
	os.WriteFile(target, []byte("old"), 0o755)

	// A download that breaks off must leave the old program alone.
	broken := io.MultiReader(strings.NewReader("half"), iotest.ErrReader(errors.New("connection lost")))
	if err := install(target, broken); err == nil {
		t.Fatal("a broken download was installed")
	}
	if data, _ := os.ReadFile(target); string(data) != "old" {
		t.Fatalf("after a broken download the program is %q, want old", data)
	}

	if err := install(target, strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); string(data) != "new" {
		t.Errorf("the program is %q, want new", data)
	}
	if info, _ := os.Stat(target); runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Error("the new program cannot be run")
	}
	left, _ := os.ReadDir(filepath.Dir(target))
	if len(left) != 1 {
		t.Errorf("%d files left behind, want only the program", len(left))
	}
}
