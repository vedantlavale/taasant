// Package tgstore stores encrypted files in a Telegram chat through the Bot API.
// See DOC.md for how it works.
package tgstore

import (
	"context"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Telegram bots can only download files up to 20 MB, so parts stay below that.
const defaultPartSize = 19 << 20

// Store must be created with New.
type Store struct {
	Endpoint string
	Token    string
	ChatID   int64
	Workers  int
	Client   *http.Client

	partSize int

	// What Readers have fetched, see reader.go.
	mu     sync.Mutex
	shapes map[string]shape
	parts  map[string]*fetched
	order  []string
}

type stored struct {
	FileID    string `json:"file_id"`
	MessageID int    `json:"message_id"`
}

type manifest struct {
	Parts []stored `json:"parts"`
}

type part struct {
	index int
	data  []byte
}

func New(token string, chatID int64) *Store {
	return &Store{
		Endpoint: "https://api.telegram.org",
		Token:    token,
		ChatID:   chatID,
		Workers:  3,
		Client:   http.DefaultClient,
		partSize: defaultPartSize,
	}
}

// Upload stores everything read from r and returns the ID needed to download it.
// The key must be 16, 24 or 32 bytes long.
//
// progress, if not nil, is called with a number of bytes each time some more
// of the object has been uploaded. The numbers add up to the size of the
// object. One can be negative: after a failed attempt a part starts again
// from zero. Calls never overlap.
func (s *Store) Upload(ctx context.Context, key []byte, r io.Reader, progress func(bytes int)) (string, error) {
	aead, err := newCipher(key)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		ids      = map[int]stored{}
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}

	parts := make(chan part)
	for range max(s.Workers, 1) {
		wg.Go(func() {
			for p := range parts {
				// counted is how much of this part progress has heard of.
				counted := 0
				report := func(done, total int) {
					now := int(int64(done) * int64(len(p.data)) / int64(total))
					mu.Lock()
					if progress != nil {
						progress(now - counted)
					}
					counted = now
					mu.Unlock()
				}
				id, err := s.sendFile(ctx, seal(aead, p.data), report)
				if err != nil {
					fail(err)
					continue
				}
				report(1, 1)
				mu.Lock()
				ids[p.index] = id
				mu.Unlock()
			}
		})
	}

	count := 0
	for ctx.Err() == nil {
		buf := make([]byte, s.partSize)
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			parts <- part{index: count, data: buf[:n]}
			count++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			fail(err)
		}
	}
	close(parts)
	wg.Wait()

	if firstErr != nil {
		return "", firstErr
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	m := manifest{Parts: make([]stored, count)}
	for i := range count {
		m.Parts[i] = ids[i]
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sent, err := s.sendFile(ctx, seal(aead, data), nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%s", sent.MessageID, sent.FileID), nil
}

// Download writes the object stored under id to w. progress works as it does
// for Upload, with the bytes downloaded.
func (s *Store) Download(ctx context.Context, key []byte, id string, w io.Writer, progress func(bytes int)) error {
	aead, err := newCipher(key)
	if err != nil {
		return err
	}
	m, _, err := s.fetchManifest(ctx, aead, id)
	if err != nil {
		return err
	}

	type result struct {
		data []byte
		err  error
	}
	results := make([]chan result, len(m.Parts))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// counted is how much of each part progress has heard of.
	var mu sync.Mutex
	counted := make([]int, len(m.Parts))
	report := func(i, now int) {
		mu.Lock()
		if progress != nil {
			progress(now - counted[i])
		}
		counted[i] = now
		mu.Unlock()
	}
	for i, p := range m.Parts {
		ch := make(chan result, 1)
		results[i] = ch
		go func() {
			data, err := s.fetchPart(ctx, aead, p.FileID, func(done, total int) { report(i, done) })
			ch <- result{data: data, err: err}
		}()
	}
	for i, ch := range results {
		result := <-ch
		if result.err != nil {
			cancel()
			return result.err
		}
		if _, err := w.Write(result.data); err != nil {
			cancel()
			return err
		}
		// What arrived was encrypted and a little larger, so settle on the real size.
		report(i, len(result.data))
	}
	return nil
}

// Delete removes the messages that hold the object stored under id.
// Telegram only lets a bot delete messages sent in the last 48 hours.
// report, if not nil, is told how many messages have gone after each one, so
// a caller can show a bar.
func (s *Store) Delete(ctx context.Context, key []byte, id string, report func(done, total int)) error {
	aead, err := newCipher(key)
	if err != nil {
		return err
	}
	m, messageID, err := s.fetchManifest(ctx, aead, id)
	if err != nil {
		return err
	}

	total := len(m.Parts) + 1
	for i, p := range m.Parts {
		if err := s.deleteMessage(ctx, p.MessageID); err != nil {
			return err
		}
		if report != nil {
			report(i+1, total)
		}
	}
	if err := s.deleteMessage(ctx, messageID); err != nil {
		return err
	}
	if report != nil {
		report(total, total)
	}
	return nil
}

func (s *Store) fetchManifest(ctx context.Context, aead cipher.AEAD, id string) (manifest, int, error) {
	var m manifest
	message, fileID, found := strings.Cut(id, ":")
	messageID, err := strconv.Atoi(message)
	if !found || err != nil {
		return m, 0, errors.New("invalid object id")
	}

	data, err := s.fetchPart(ctx, aead, fileID, nil)
	if err != nil {
		return m, 0, err
	}
	err = json.Unmarshal(data, &m)
	return m, messageID, err
}

func (s *Store) fetchPart(ctx context.Context, aead cipher.AEAD, id string, received func(done, total int)) ([]byte, error) {
	data, err := s.fetchFile(ctx, id, received)
	if err != nil {
		return nil, err
	}
	return open(aead, data)
}
