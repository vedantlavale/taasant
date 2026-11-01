package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"time"
)

const usage = `usage:
  taasant                 open the menu
  taasant keygen
  taasant upload [--to <place>] <file>
  taasant list [search]
  taasant download <name or id> [output-file]
  taasant delete <name or id>
  taasant mount           show the files as a drive in Finder
  taasant autostart [off] show the drive at every login, or stop doing that
  taasant update          get the newest version
  taasant version

Add --json to list, upload, download or delete to get lines of JSON, which is
what the Mac app reads: "list" gives all files as one array, and an upload or
download gives {"done":..,"total":..} as it goes.

A name can be typed partly: "taasant download hol" finds "holiday.jpg".

Plain "taasant" asks for the settings the first time and saves them.

environment, each one optional and stronger than the saved settings:
  TG_BOT_TOKEN  bot token from @BotFather
  TG_CHAT_ID    chat the bot stores files in
  TAAS_KEY      encryption key printed by "taasant keygen"
  TAAS_CONFIG   where the settings are kept
  TAAS_INDEX    where the list of uploads is kept`

var errUsage = errors.New(usage)

func main() {
	args := os.Args[1:]
	var err error
	if len(args) == 0 && outTTY && isTerminal(os.Stdin) {
		err = menu()
	} else {
		err = run(args)
	}
	if err == errUsage {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		printError(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errUsage
	}
	command, args := args[0], args[1:]
	asJSON := slices.Contains(args, "--json")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--json" })
	to := ""
	if i := slices.Index(args, "--to"); i >= 0 {
		if command != "upload" || i+1 == len(args) {
			return errUsage
		}
		to = args[i+1]
		args = slices.Delete(slices.Clone(args), i, i+2)
	}

	switch {
	case command == "keygen" && len(args) == 0:
		key := make([]byte, 32)
		rand.Read(key)
		fmt.Println(hex.EncodeToString(key))
		return nil
	case command == "help":
		fmt.Println(usage)
		return nil
	case command == "version" && len(args) == 0:
		fmt.Println("taasant", version())
		return nil
	case command == "update" && len(args) == 0:
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return update(ctx)
	case command == "mount" && len(args) == 0:
		return mount(false)
	case command == "mount" && len(args) == 1 && args[0] == "background":
		return mount(true)
	case command == "autostart" && len(args) == 0:
		return autostart(true)
	case command == "autostart" && len(args) == 1 && args[0] == "off":
		return autostart(false)
	case command == "list" && len(args) <= 1:
	case command == "upload" && len(args) == 1:
	case command == "download" && (len(args) == 1 || len(args) == 2):
	case command == "delete" && len(args) == 1:
	default:
		return errUsage
	}

	indexFile, err := indexPath()
	if err != nil {
		return err
	}
	entries, err := loadIndex(indexFile)
	if err != nil {
		return err
	}

	if command == "list" {
		if asJSON {
			return printJSON(append([]entry{}, entries...))
		}
		return list(entries, args)
	}

	v, err := connect()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := time.Now()

	switch command {
	case "upload":
		added, err := addFile(ctx, v, to, indexFile, args[0], progressFor(filepath.Base(args[0]), asJSON))
		clearProgress()
		if err != nil {
			return err
		}
		if asJSON {
			printJSON(added)
		} else if outTTY {
			details := formatSize(added.Size) + "  uploaded in " + formatDuration(time.Since(started))
			printDone(added.Name + "  " + paint(true, dim, details))
		} else {
			fmt.Println(added.ID)
		}
		return backupIndex(ctx, v, indexFile)

	case "download":
		e, err := find(entries, args[0])
		if err != nil {
			return err
		}
		output := e.Name
		if len(args) == 2 {
			output = args[1]
		} else if output != "" {
			// A file in a folder of the drive has the folder in its name.
			output = path.Base(output)
		}
		if output == "" {
			return errors.New("this ID is not in the index, so give an output file name")
		}
		err = download(ctx, v, e, output, progressFor(output, asJSON))
		clearProgress()
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(map[string]string{"saved": output})
		}
		printDone("saved " + output)
		return nil

	default:
		e, err := find(entries, args[0])
		if err != nil {
			return err
		}
		if err := removeFile(ctx, v, e, indexFile); err != nil {
			return err
		}
		if asJSON {
			printJSON(map[string]string{"deleted": e.ID})
		} else {
			printDone("deleted " + cmp.Or(e.Name, e.ID))
		}
		return backupIndex(ctx, v, indexFile)
	}
}

// connect opens the places files can go, out of the settings.
func connect() (*vault, error) {
	c, err := settings()
	if err != nil {
		return nil, err
	}
	return newVault(c), nil
}

func list(entries []entry, args []string) error {
	query := ""
	if len(args) == 1 {
		query = args[0]
	}
	found := search(entries, query)
	switch {
	case len(entries) == 0:
		fmt.Println(paint(outTTY, dim, "  no files yet, add one with: taasant upload <file>"))
	case len(found) == 0:
		return fmt.Errorf("nothing in the index matches %q", query)
	default:
		printList(found, time.Now())
	}
	return nil
}

// The three functions below do the real work, for the commands and for the
// menu alike. They print nothing. report is called as parts finish, with the
// bytes done so far and the total.

// addFile uploads the file at path and writes it into the index.
func addFile(ctx context.Context, v *vault, to, indexFile, path string, report func(done, total int64)) (entry, error) {
	store, err := v.get(to)
	if err != nil {
		return entry{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return entry{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return entry{}, err
	}
	if info.IsDir() {
		return entry{}, fmt.Errorf("%s is a folder, upload one file at a time", path)
	}

	var sent int64
	report(0, info.Size())
	id, err := store.Upload(ctx, v.key, file, func(bytes int) {
		sent += int64(bytes)
		report(sent, info.Size())
	})
	if err != nil {
		return entry{}, err
	}
	added := entry{Name: filepath.Base(path), ID: id, Backend: cmp.Or(to, v.def), Size: info.Size(), Uploaded: time.Now()}

	entries, err := loadIndex(indexFile)
	if err == nil {
		err = saveIndex(indexFile, append(entries, added))
	}
	if err != nil {
		return added, fmt.Errorf("uploaded as %s, but the index could not be saved: %w", id, err)
	}
	return added, nil
}

// download refuses to overwrite, so a failed download can never destroy an existing file.
func download(ctx context.Context, v *vault, e entry, output string, report func(done, total int64)) error {
	store, err := v.get(e.where())
	if err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}

	var received int64
	report(0, e.Size)
	err = store.Download(ctx, v.key, e.ID, file, func(bytes int) {
		received += int64(bytes)
		report(received, e.Size)
	})
	if err != nil {
		file.Close()
		os.Remove(output)
		return err
	}
	return file.Close()
}

// removeFile deletes a stored file and takes it out of the index.
func removeFile(ctx context.Context, v *vault, e entry, indexFile string) error {
	store, err := v.get(e.where())
	if err != nil {
		return err
	}
	if err := store.Delete(ctx, v.key, e.ID); err != nil {
		return err
	}
	entries, err := loadIndex(indexFile)
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(entries, func(other entry) bool { return other.ID == e.ID })
	return saveIndex(indexFile, entries)
}

func backupIndex(ctx context.Context, v *vault, path string) error {
	store, err := v.get("")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	id, err := store.Upload(ctx, v.key, bytes.NewReader(data), nil)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".remote", []byte(id+"\n"), 0o600)
}

// progressFor shows the progress of one file: a bar for a person, or a line
// of JSON each time a whole percent has gone by for a program.
func progressFor(name string, asJSON bool) func(done, total int64) {
	if !asJSON {
		return showProgress(name)
	}
	last := int64(-1)
	return func(done, total int64) {
		if percent := done * 100 / max(total, 1); percent != last {
			last = percent
			printJSON(map[string]int64{"done": done, "total": total})
		}
	}
}

func printJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}
