package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	latestRelease = "https://api.github.com/repos/vedantlavale/taasant/releases/latest"
	downloads     = "https://github.com/vedantlavale/taasant/releases/download"
)

// The file each kind of computer downloads from a release.
var assets = map[string]string{
	"darwin/arm64":  "taasant-macos-arm64",
	"darwin/amd64":  "taasant-macos-intel",
	"linux/amd64":   "taasant-linux-amd64",
	"windows/amd64": "taasant-windows.exe",
}

// version is the release this program was built from, like "v0.2.0". Go
// writes it into the program when it is built from a tagged commit.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// update replaces this program with the newest release.
func update(ctx context.Context) error {
	var release struct {
		Tag string `json:"tag_name"`
	}
	body, err := get(ctx, latestRelease)
	if err != nil {
		return err
	}
	err = json.NewDecoder(body).Decode(&release)
	body.Close()
	if err != nil || release.Tag == "" {
		return errors.New("could not read the newest version from GitHub")
	}
	if release.Tag == version() {
		printDone("taasant " + release.Tag + " is the newest version")
		return nil
	}

	asset, found := assets[runtime.GOOS+"/"+runtime.GOARCH]
	if !found {
		return errors.New("there is no download for this kind of computer, use: go install github.com/vedantlavale/taasant/cmd/taasant@latest")
	}
	// The program may have been started through a link, so find the real file.
	target, err := os.Executable()
	if err == nil {
		target, err = filepath.EvalSymlinks(target)
	}
	if err != nil {
		return err
	}
	// Homebrew and Scoop keep their own record of the version they installed,
	// so they have to do the updating.
	if strings.Contains(target, "/Cellar/") {
		return errors.New("this taasant was installed with Homebrew, update it with: brew upgrade taasant")
	}
	if strings.Contains(target, `\scoop\`) {
		return errors.New("this taasant was installed with Scoop, update it with: scoop update taasant")
	}

	body, err = get(ctx, downloads+"/"+release.Tag+"/"+asset)
	if err != nil {
		return err
	}
	defer body.Close()
	err = install(target, body)
	clearProgress()
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("no permission to write in %s, try: sudo taasant update", filepath.Dir(target))
	}
	if err != nil {
		return err
	}
	printDone("updated " + version() + " → " + release.Tag)
	return nil
}

// get fetches url and returns what came back. A reply that is not "200 OK"
// is an error.
func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, res.Status)
	}
	report := showProgress(filepath.Base(url))
	return &counted{ReadCloser: res.Body, total: res.ContentLength, report: report}, nil
}

// counted passes a download through and reports how far it is.
type counted struct {
	io.ReadCloser
	done, total int64
	report      func(done, total int64)
}

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.done += int64(n)
	if c.total > 1<<20 {
		c.report(c.done, c.total)
	}
	return n, err
}

// install writes the new program next to the old one first, and only swaps
// them once it is complete. A download that breaks off leaves the old
// program as it was.
func install(target string, program io.Reader) error {
	fresh := target + ".new"
	file, err := os.OpenFile(fresh, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, program)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(fresh)
		return err
	}

	// Windows will not overwrite a program that is running, but it lets you
	// rename it. So the old one steps aside first.
	old := target + ".old"
	os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		os.Remove(fresh)
		return err
	}
	if err := os.Rename(fresh, target); err != nil {
		os.Rename(old, target)
		return err
	}
	os.Remove(old)
	return nil
}
