package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/vedantlavale/taasant/tgstore"
)

const usage = `usage:
  taasant                 open the menu
  taasant keygen
  taasant upload <file>
  taasant list [search]
  taasant download <name or id> [output-file]
  taasant delete <name or id>
  taasant update          get the newest version
  taasant version

A name can be typed partly: "taasant download hol" finds "holiday.jpg".

environment:
  TG_BOT_TOKEN  bot token from @BotFather
  TG_CHAT_ID    chat the bot stores files in
  TAAS_KEY      encryption key printed by "taasant keygen"
  TAAS_INDEX    optional, where the list of uploads is kept`

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
		return list(entries, args)
	}

	store, key, err := connect()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	started := time.Now()

	switch command {
	case "upload":
		added, err := addFile(ctx, store, key, indexFile, args[0], showProgress(filepath.Base(args[0])))
		clearProgress()
		if err != nil {
			return err
		}
		if outTTY {
			details := formatSize(added.Size) + "  uploaded in " + formatDuration(time.Since(started))
			printDone(added.Name + "  " + paint(true, dim, details))
		} else {
			fmt.Println(added.ID)
		}
		return backupIndex(ctx, store, key, indexFile)

	case "download":
		e, err := find(entries, args[0])
		if err != nil {
			return err
		}
		output := e.Name
		if len(args) == 2 {
			output = args[1]
		}
		if output == "" {
			return errors.New("this ID is not in the index, so give an output file name")
		}
		err = download(ctx, store, key, e, output, showProgress(output))
		clearProgress()
		if err != nil {
			return err
		}
		printDone("saved " + output)
		return nil

	default:
		e, err := find(entries, args[0])
		if err != nil {
			return err
		}
		if err := removeFile(ctx, store, key, indexFile, e); err != nil {
			return err
		}
		printDone("deleted " + cmp.Or(e.Name, e.ID))
		return backupIndex(ctx, store, key, indexFile)
	}
}

// connect reads the bot's settings and the encryption key from the environment.
func connect() (*tgstore.Store, []byte, error) {
	chatID, err := strconv.ParseInt(os.Getenv("TG_CHAT_ID"), 10, 64)
	if err != nil {
		return nil, nil, errors.New("TG_CHAT_ID must be a number")
	}
	token := os.Getenv("TG_BOT_TOKEN")
	if token == "" {
		return nil, nil, errors.New("TG_BOT_TOKEN is not set")
	}
	key, err := hex.DecodeString(os.Getenv("TAAS_KEY"))
	if err != nil || len(key) != 32 {
		return nil, nil, errors.New(`TAAS_KEY must be 64 hex characters, create one with "taasant keygen"`)
	}
	return tgstore.New(token, chatID), key, nil
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
func addFile(ctx context.Context, store *tgstore.Store, key []byte, indexFile, path string, report func(done, total int64)) (entry, error) {
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
	store.Progress = func(bytes int) {
		sent += int64(bytes)
		report(sent, info.Size())
	}
	id, err := store.Upload(ctx, key, file)
	store.Progress = nil
	if err != nil {
		return entry{}, err
	}
	added := entry{Name: filepath.Base(path), ID: id, Size: info.Size(), Uploaded: time.Now()}

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
func download(ctx context.Context, store *tgstore.Store, key []byte, e entry, output string, report func(done, total int64)) error {
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}

	var received int64
	report(0, e.Size)
	store.Progress = func(bytes int) {
		received += int64(bytes)
		report(received, e.Size)
	}
	err = store.Download(ctx, key, e.ID, file)
	store.Progress = nil
	if err != nil {
		file.Close()
		os.Remove(output)
		return err
	}
	return file.Close()
}

// removeFile deletes a stored file and takes it out of the index.
func removeFile(ctx context.Context, store *tgstore.Store, key []byte, indexFile string, e entry) error {
	if err := store.Delete(ctx, key, e.ID); err != nil {
		return err
	}
	entries, err := loadIndex(indexFile)
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(entries, func(other entry) bool { return other.ID == e.ID })
	return saveIndex(indexFile, entries)
}

func backupIndex(ctx context.Context, store *tgstore.Store, key []byte, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	id, err := store.Upload(ctx, key, bytes.NewReader(data))
	if err != nil {
		return err
	}
	return os.WriteFile(path+".remote", []byte(id+"\n"), 0o600)
}
