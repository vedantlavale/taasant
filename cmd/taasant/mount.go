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
	drivePort = "8738"
)

// mount shows the drive in Finder. In the background, used by autostart, no
// window is left open.
func mount(background bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("mount puts the drive in Finder, so it needs a Mac")
	}
	indexFile, err := indexPath()
	if err != nil {
		return err
	}
	v, err := connect()
	if err != nil {
		return err
	}
	d := newDrive(v, indexFile)
	d.progress.path = indexFile + ".progress"
	defer os.Remove(d.progress.path)
	d.note = func(text string) { fmt.Println(paint(outTTY, dim, "  "+text)) }
	// Whoever is copying files in Finder is not looking at this window, so
	// a file that was not saved is also shown as a notification.
	d.alert = func(text string) {
		printError(errors.New(text))
		exec.Command("osascript", "-e", "on run argv",
			"-e", `display notification (item 1 of argv) with title "taasant"`,
			"-e", "end run", "--", text).Run()
	}
	return d.mount(background)
}

// showWindow is the AppleScript that opens the drive in a Finder window.
// Finder draws its icon previews by reading the files, which downloads the
// start of every file in the window. So the window is told to show plain icons.
// When the second argument is "hide" the window is closed again at once: the
// drive appears at login, and nobody wants a window for it then.
const showWindow = `on run argv
	tell application "Finder"
		set w to make new Finder window to (POSIX file (item 1 of argv) as alias)
		set shows icon preview of icon view options of w to false
		set shows icon preview of list view options of w to false
		set shows icon preview of column view options of w to false
		set shows preview column of column view options of w to false
		if (item 2 of argv) is "hide" then
			close w
		else
			activate
		end if
	end tell
end run`

// mount puts the drive in Finder, until Ctrl-C or until it is ejected there.
func (d *drive) mount(background bool) error {
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
	printDone("the drive is in Finder, under Locations. Ctrl-C ejects it")
	// Not waited for: the first time, macOS asks whether this program may
	// control Finder, and the drive must work before that is answered.
	go func() {
		mode := "show"
		if background {
			mode = "hide"
		}
		if exec.Command("osascript", "-e", showWindow, "--", folder, mode).Run() != nil && !background {
			exec.Command("open", folder).Run()
		}
	}()

	force := false
	for mounted := true; mounted; {
		select {
		case <-ctx.Done():
			stop()
			switch {
			case exec.Command("umount", folder).Run() == nil:
				mounted = false
			case force:
				exec.Command("diskutil", "unmount", "force", folder).Run()
				mounted = false
			default:
				// A file is open, or one is still on its way to Telegram
				// and would be lost.
				printError(errors.New("the drive is busy, a file is open or still being saved. Ctrl-C again ejects it anyway"))
				force = true
				ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			}
		case <-time.After(time.Second):
			// The folder goes away when the drive is ejected in Finder.
			_, err := os.Stat(folder)
			mounted = err == nil
		}
	}

	// From here Ctrl-C ends taasant at once.
	stop()
	server.Shutdown(context.Background())
	if !d.changed {
		return nil
	}
	fmt.Println(paint(outTTY, dim, "  saving the list of files to the chat"))
	return backupIndex(context.Background(), d.vault, d.indexFile)
}
