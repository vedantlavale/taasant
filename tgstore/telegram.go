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

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (s *Store) call(ctx context.Context, method, contentType string, body []byte, result any) error {
	target := s.Endpoint + "/bot" + s.Token + "/" + method

	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", contentType)

		res, err := s.Client.Do(req)
		if err != nil {
			return err
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

func (s *Store) sendFile(ctx context.Context, data []byte) (stored, error) {
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
	err = s.call(ctx, "sendDocument", form.FormDataContentType(), body.Bytes(), &message)
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
	return s.call(ctx, "deleteMessage", "application/x-www-form-urlencoded", []byte(query), &deleted)
}

func (s *Store) fetchFile(ctx context.Context, id string) ([]byte, error) {
	var file struct {
		FilePath string `json:"file_path"`
	}
	query := url.Values{"file_id": {id}}.Encode()
	err := s.call(ctx, "getFile", "application/x-www-form-urlencoded", []byte(query), &file)
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
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram file download: %s", res.Status)
	}
	return io.ReadAll(res.Body)
}
