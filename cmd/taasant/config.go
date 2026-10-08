package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// config is what a device needs to reach the files: the bot, the chat it
// posts in and the encryption key.
type config struct {
	Token  string `json:"token"`
	ChatID int64  `json:"chat_id"`
	Key    string `json:"key"`
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
	if token := os.Getenv("TG_BOT_TOKEN"); token != "" {
		c.Token = token
	}
	if chat := os.Getenv("TG_CHAT_ID"); chat != "" {
		if c.ChatID, err = strconv.ParseInt(chat, 10, 64); err != nil {
			return config{}, errors.New("TG_CHAT_ID must be a number")
		}
	}
	if key := os.Getenv("TAAS_KEY"); key != "" {
		c.Key = key
	}
	return c, c.check()
}

func (c config) check() error {
	key, err := hex.DecodeString(c.Key)
	switch {
	case c.Token == "":
		return errors.New(`there is no bot token, run "taasant" in a terminal to set up`)
	case c.ChatID == 0:
		return errors.New(`there is no chat, run "taasant" in a terminal to set up`)
	case err != nil || len(key) != 32:
		return errors.New(`the key must be 64 hex characters, create one with "taasant keygen"`)
	}
	return nil
}
