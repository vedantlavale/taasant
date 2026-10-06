package main

import (
	"bytes"
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
  taasant keygen
  taasant upload <file>
  taasant list [search]
  taasant download <name or id> [output-file]
  taasant delete <name or id>

A name can be typed partly: "taasant download hol" finds "holiday.jpg".

environment:
  TG_BOT_TOKEN  bot token from @BotFather
  TG_CHAT_ID    chat the bot stores files in
  TAAS_KEY      encryption key printed by "taasant keygen"
  TAAS_INDEX    optional, where the list of uploads is kept`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	command, args := args[0], args[1:]

	switch {
	case command == "keygen" && len(args) == 0:
		key := make([]byte, 32)
		rand.Read(key)
		fmt.Println(hex.EncodeToString(key))
		return nil
	case command == "list" && len(args) <= 1:
	case command == "upload" && len(args) == 1:
	case command == "download" && (len(args) == 1 || len(args) == 2):
	case command == "delete" && len(args) == 1:
	default:
		return errors.New(usage)
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
		query := ""
		if len(args) == 1 {
			query = args[0]
		}
		for _, e := range search(entries, query) {
			fmt.Println(e)
		}
		return nil
	}

	chatID, err := strconv.ParseInt(os.Getenv("TG_CHAT_ID"), 10, 64)
	if err != nil {
		return errors.New("TG_CHAT_ID must be a number")
	}
	token := os.Getenv("TG_BOT_TOKEN")
	if token == "" {
		return errors.New("TG_BOT_TOKEN is not set")
	}
	key, err := hex.DecodeString(os.Getenv("TAAS_KEY"))
	if err != nil || len(key) != 32 {
		return errors.New(`TAAS_KEY must be 64 hex characters, create one with "taasant keygen"`)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	store := tgstore.New(token, chatID)

	switch command {
	case "upload":
		added, err := upload(ctx, store, key, args[0])
		if err != nil {
			return err
		}
		fmt.Println(added.ID)
		if err := saveIndex(indexFile, append(entries, added)); err != nil {
			return err
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
		if err := download(ctx, store, key, e.ID, output); err != nil {
			return err
		}
		fmt.Println("saved", output)
		return nil

	default:
		e, err := find(entries, args[0])
		if err != nil {
			return err
		}
		if err := store.Delete(ctx, key, e.ID); err != nil {
			return err
		}
		entries = slices.DeleteFunc(entries, func(other entry) bool { return other.ID == e.ID })
		if err := saveIndex(indexFile, entries); err != nil {
			return err
		}
		return backupIndex(ctx, store, key, indexFile)
	}
}

func upload(ctx context.Context, store *tgstore.Store, key []byte, path string) (entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return entry{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return entry{}, err
	}

	id, err := store.Upload(ctx, key, file)
	if err != nil {
		return entry{}, err
	}
	return entry{Name: filepath.Base(path), ID: id, Size: info.Size(), Uploaded: time.Now()}, nil
}

// download refuses to overwrite, so a failed download can never destroy an existing file.
func download(ctx context.Context, store *tgstore.Store, key []byte, id, output string) error {
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := store.Download(ctx, key, id, file); err != nil {
		file.Close()
		os.Remove(output)
		return err
	}
	return file.Close()
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
