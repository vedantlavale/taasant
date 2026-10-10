package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Finder shows the drive under driveName. The port is always the same, so a
// second "taasant mount" finds it taken and stops.
const (
	driveName = "taasant"
	drivePort = "8737"
)

func mount() error {
	if runtime.GOOS != "darwin" {
		return errors.New("mount puts the drive in Finder, so it needs a Mac")
	}
	indexFile, err := indexPath()
	if err != nil {
		return err
	}
	store, key, err := connect()
	if err != nil {
		return err
	}
	d := newDrive(store, key, indexFile)
	d.note = func(text string) { fmt.Println(paint(outTTY, dim, "  "+text)) }
	return d.mount()
}

// mount puts the drive in Finder, until Ctrl-C or until it is ejected there.
func (d *drive) mount() error {
	// Finder lists the drive under the name of the computer it is on, and
	// any name that ends in .localhost is this computer.
	host := driveName + ".localhost"
	if _, err := net.LookupHost(host); err != nil {
		host = "localhost"
	}
	host += ":" + drivePort

	listener, err := net.Listen("tcp", "127.0.0.1:"+drivePort)
	if err != nil {
		return fmt.Errorf("the drive could not start, is it mounted already? %w", err)
	}
	server := &http.Server{Handler: d.handler(host)}
	go server.Serve(listener)
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	script := fmt.Sprintf(`POSIX path of (mount volume "http://%s/%s")`, host, driveName)
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("the drive could not be put in Finder: %s", bytes.TrimSpace(out))
	}
	folder := strings.TrimSpace(string(out))
	exec.Command("open", folder).Run()
	printDone("the drive is in Finder, under Locations. Ctrl-C ejects it")

	for mounted := true; mounted; {
		select {
		case <-ctx.Done():
			// By force if something still has a file open.
			if exec.Command("umount", folder).Run() != nil {
				exec.Command("diskutil", "unmount", "force", folder).Run()
			}
			mounted = false
		case <-time.After(time.Second):
			// The folder goes away when the drive is ejected in Finder.
			_, err := os.Stat(folder)
			mounted = err == nil
		}
	}

	// From here a second Ctrl-C ends taasant at once.
	stop()
	server.Shutdown(context.Background())
	if !d.changed {
		return nil
	}
	fmt.Println(paint(outTTY, dim, "  saving the list of files to the chat"))
	return backupIndex(context.Background(), d.store, d.key, d.indexFile)
}
