package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// config is what a device needs to reach the files: the places they can be
// stored, which one is the default, and the encryption key.
type config struct {
	Key      string            `json:"key"`
	Default  string            `json:"default,omitempty"`
	Backends map[string]target `json:"backends,omitempty"`

	// Before there could be more than one place, the bot and the chat were
	// kept here. They are only read, see migrate.
	Token  string `json:"token,omitempty"`
	ChatID int64  `json:"chat_id,omitempty"`
}

// target is one place to store files. Type says which of the fields apply:
// "telegram" uses Token and ChatID, "s3" uses the rest. R2, AWS and MinIO are
// all "s3", they differ only in these values.
type target struct {
	Type string `json:"type"`

	Token  string `json:"token,omitempty"`
	ChatID int64  `json:"chat_id,omitempty"`

	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"`
}

// migrate moves a bot and a chat from an old settings file into a backend.
func (c *config) migrate() {
	if c.Token == "" && c.ChatID == 0 {
		return
	}
	if _, found := c.Backends["telegram"]; !found {
		c.set("telegram", target{Type: "telegram", Token: c.Token, ChatID: c.ChatID})
	}
	c.Token, c.ChatID = "", 0
}

// set adds a backend, and makes it the default if there was none.
func (c *config) set(name string, t target) {
	if c.Backends == nil {
		c.Backends = map[string]target{}
	}
	c.Backends[name] = t
	if c.Default == "" {
		c.Default = name
	}
}

func configPath() (string, error) {
	if path := os.Getenv("TAAS_CONFIG"); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "taas", "config.json"), nil
}

// loadConfig returns an empty config when there is no file yet.
func loadConfig(path string) (config, error) {
	var c config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &c)
	}
	c.migrate()
	return c, err
}

func saveConfig(path string, c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

// writeFile writes to a temporary file first, so a crash cannot leave a half-written file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// settings reads the config file. The environment variables win over it.
func settings() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, err
	}
	c, err := loadConfig(path)
	if err != nil {
		return config{}, err
	}
	token, chat := os.Getenv("TG_BOT_TOKEN"), os.Getenv("TG_CHAT_ID")
	if token != "" || chat != "" {
		t := c.Backends["telegram"]
		t.Type = "telegram"
		if token != "" {
			t.Token = token
		}
		if chat != "" {
			if t.ChatID, err = strconv.ParseInt(chat, 10, 64); err != nil {
				return config{}, errors.New("TG_CHAT_ID must be a number")
			}
		}
		c.set("telegram", t)
	}
	if key := os.Getenv("TAAS_KEY"); key != "" {
		c.Key = key
	}
	return c, c.check()
}

// check says whether the default backend and the key are complete.
func (c config) check() error {
	key, err := hex.DecodeString(c.Key)
	t, found := c.Backends[c.Default]
	switch {
	case !found:
		return errors.New(`there is nowhere to store files yet, run "taasant" in a terminal to set up`)
	case t.check() != nil:
		return t.check()
	case err != nil || len(key) != 32:
		return errors.New(`the key must be 64 hex characters, create one with "taasant keygen"`)
	}
	return nil
}

func (t target) check() error {
	switch t.Type {
	case "telegram":
		switch {
		case t.Token == "":
			return errors.New(`there is no bot token, run "taasant" in a terminal to set up`)
		case t.ChatID == 0:
			return errors.New(`there is no chat, run "taasant" in a terminal to set up`)
		}
	case "s3":
		if !strings.HasPrefix(t.Endpoint, "https://") && !strings.HasPrefix(t.Endpoint, "http://") {
			return errors.New("the endpoint of the storage must start with https://")
		}
		for _, f := range []struct{ name, value string }{
			{"endpoint", t.Endpoint}, {"region", t.Region}, {"bucket", t.Bucket},
			{"access key", t.AccessKey}, {"secret key", t.SecretKey},
		} {
			if f.value == "" {
				return fmt.Errorf(`there is no %s for the storage, run "taasant" in a terminal to set up`, f.name)
			}
		}
	default:
		return fmt.Errorf("%q is not a kind of storage taasant knows", t.Type)
	}
	return nil
}
