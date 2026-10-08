package tgstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const maxAttempts = 5

// retryWait is the pause before trying again after a broken connection.
var retryWait = time.Second

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call sends one request to Telegram and tries again when that can help: the
// connection broke, Telegram is busy, or it asked us to slow down. sent, if
// not nil, is told how much of the body has gone out so far; after a failed
// attempt the count starts again from zero.
func (s *Store) call(ctx context.Context, method, contentType string, body []byte, sent func(done, total int), result any) error {
	target := s.Endpoint + "/bot" + s.Token + "/" + method

	for attempt := 1; ; attempt++ {
		var reader io.Reader = bytes.NewReader(body)
		if sent != nil {
			reader = &countingReader{reader: reader, total: len(body), report: sent}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, reader)
		if err != nil {
			return err
		}
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", contentType)

		res, err := s.Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt == maxAttempts {
				return fmt.Errorf("telegram %s: %w", method, withoutURL(err))
			}
			select {
			case <-time.After(retryWait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		var r apiResponse
		err = json.NewDecoder(res.Body).Decode(&r)
		res.Body.Close()
		if err != nil && res.StatusCode < 500 {
			return err
		}
		if r.OK {
			return json.Unmarshal(r.Result, result)
		}

		retryable := res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500
		if !retryable || attempt == maxAttempts {
			return fmt.Errorf("telegram %s: %s %s", method, res.Status, r.Description)
		}

		wait := time.Second
		if r.Parameters.RetryAfter > 0 {
			wait = time.Duration(r.Parameters.RetryAfter) * time.Second
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// withoutURL drops the address from a network error. The address has the
// bot token in it, which must not end up on screen or in a bug report.
func withoutURL(err error) error {
	var failed *url.Error
	if errors.As(err, &failed) {
		return failed.Err
	}
	return err
}

// countingReader reports how much has been read through it.
type countingReader struct {
	reader      io.Reader
	done, total int
	report      func(done, total int)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.done += n
	c.report(c.done, c.total)
	return n, err
}

func (s *Store) sendFile(ctx context.Context, data []byte, sent func(done, total int)) (stored, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("chat_id", strconv.FormatInt(s.ChatID, 10))
	form.WriteField("disable_notification", "true")
	file, err := form.CreateFormFile("document", "part.bin")
	if err != nil {
		return stored{}, err
	}
	file.Write(data)
	form.Close()

	var message struct {
		MessageID int `json:"message_id"`
		Document  struct {
			FileID string `json:"file_id"`
		} `json:"document"`
	}
	err = s.call(ctx, "sendDocument", form.FormDataContentType(), body.Bytes(), sent, &message)
	if err != nil {
		return stored{}, err
	}
	if message.Document.FileID == "" {
		return stored{}, errors.New("telegram sendDocument: no file id in response")
	}
	return stored{FileID: message.Document.FileID, MessageID: message.MessageID}, nil
}

func (s *Store) deleteMessage(ctx context.Context, messageID int) error {
	query := url.Values{
		"chat_id":    {strconv.FormatInt(s.ChatID, 10)},
		"message_id": {strconv.Itoa(messageID)},
	}.Encode()
	var deleted bool
	return s.call(ctx, "deleteMessage", formType, []byte(query), nil, &deleted)
}

func (s *Store) fetchFile(ctx context.Context, id string, received func(done, total int)) ([]byte, error) {
	var file struct {
		FilePath string `json:"file_path"`
	}
	query := url.Values{"file_id": {id}}.Encode()
	err := s.call(ctx, "getFile", formType, []byte(query), nil, &file)
	if err != nil {
		return nil, err
	}

	target := s.Endpoint + "/file/bot" + s.Token + "/" + file.FilePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram file download: %w", withoutURL(err))
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram file download: %s", res.Status)
	}
	var body io.Reader = res.Body
	if received != nil {
		body = &countingReader{reader: body, total: int(res.ContentLength), report: received}
	}
	return io.ReadAll(body)
}

const formType = "application/x-www-form-urlencoded"

// Chat is a place a bot can post in. Title is set for channels and groups,
// FirstName for a person.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	FirstName string `json:"first_name"`
}

// Me asks Telegram who the token belongs to and returns the bot's username.
func (s *Store) Me(ctx context.Context) (string, error) {
	var bot struct {
		Username string `json:"username"`
	}
	err := s.call(ctx, "getMe", formType, nil, nil, &bot)
	return bot.Username, err
}

// FindChat waits until the bot hears from a chat and returns it: a person
// writing to the bot if private is set, otherwise a channel or group the bot
// was added to or that got a new post. Of several it returns the newest.
//
// Telegram numbers what a bot hears. FindChat skips everything up to after
// and returns the number it got to, so the next search only hears newer things.
func (s *Store) FindChat(ctx context.Context, private bool, after int) (Chat, int, error) {
	type heard struct {
		Chat Chat `json:"chat"`
	}
	for {
		var updates []struct {
			ID      int    `json:"update_id"`
			Message *heard `json:"message"`
			Post    *heard `json:"channel_post"`
			Member  *heard `json:"my_chat_member"`
		}
		// timeout makes Telegram hold the request open until there is news.
		query := url.Values{"timeout": {"20"}, "offset": {strconv.Itoa(after + 1)}}.Encode()
		if err := s.call(ctx, "getUpdates", formType, []byte(query), nil, &updates); err != nil {
			return Chat{}, after, err
		}
		var found *Chat
		for _, u := range updates {
			after = u.ID
			for _, h := range []*heard{u.Message, u.Post, u.Member} {
				if h != nil && (h.Chat.Type == "private") == private {
					found = &h.Chat
				}
			}
		}
		if found != nil {
			return *found, after, nil
		}
	}
}
