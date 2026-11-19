package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// agentName is the name launchd knows the login job by.
const agentName = "com.taasant.drive"

// agentPlist is the job launchd runs at login: "taasant mount background".
// It is not started again after the drive is ejected, so an eject lasts until
// the next login. Aqua keeps it to the logged-in desktop, the only place
// Finder can be asked to mount anything.
func agentPlist(program, log string) []byte {
	var b bytes.Buffer
	text := func(s string) string {
		var e bytes.Buffer
		xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>mount</string>
		<string>background</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
`, agentName, text(program), text(log), text(log))
	// The settings may live somewhere else than the usual place.
	var env bytes.Buffer
	for _, name := range []string{"TAAS_CONFIG", "TAAS_INDEX"} {
		if value := os.Getenv(name); value != "" {
			// The job starts in /, so a relative path would mean another file.
			if abs, err := filepath.Abs(value); err == nil {
				value = abs
			}
			fmt.Fprintf(&env, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", name, text(value))
		}
	}
	if env.Len() > 0 {
		fmt.Fprintf(&b, "\t<key>EnvironmentVariables</key>\n\t<dict>\n%s\t</dict>\n", env.String())
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// autostart makes the drive appear at every login, or stops that.
func autostart(on bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("autostart starts the drive in Finder, so it needs a Mac")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", agentName+".plist")
	domain := "gui/" + strconv.Itoa(os.Getuid())

	// Taken out first, so that this also replaces an older copy of the job.
	// That ends a drive that is mounted by it, which is what "off" should do.
	exec.Command("launchctl", "bootout", domain+"/"+agentName).Run()
	// The job only goes once its program has ended, which takes a moment
	// while it ejects the drive and saves the list of files.
	for range 150 {
		if exec.Command("launchctl", "print", domain+"/"+agentName).Run() != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !on {
		if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
			return err
		}
		printDone("the drive no longer starts at login")
		return nil
	}

	if _, err := savedSettings(); err != nil {
		return err
	}
	program, err := os.Executable()
	if err != nil {
		return err
	}
	log := filepath.Join(home, "Library", "Logs", "taasant.log")
	if err := writeFile(plist, agentPlist(program, log)); err != nil {
		return err
	}
	if out, err := exec.Command("launchctl", "bootstrap", domain, plist).CombinedOutput(); err != nil {
		return fmt.Errorf("the drive could not be set to start at login: %s", bytes.TrimSpace(out))
	}
	printDone("the drive is in Finder now and will be at every login")
	fmt.Println(paint(outTTY, dim, "  taasant autostart off stops that. Problems are written to "+log))
	return nil
}
