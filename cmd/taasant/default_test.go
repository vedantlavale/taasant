package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("TAAS_CONFIG", path)
	c := config{
		Key:     strings.Repeat("ab", 32),
		Default: "r2",
		Backends: map[string]target{
			"r2":       {Type: "s3", Endpoint: "https://x", Bucket: "bucket", AccessKey: "a", SecretKey: "s"},
			"telegram": {Type: "telegram", Token: "TOKEN", ChatID: -100},
		},
	}
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}

	if err := setDefault("telegram"); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Default != "telegram" {
		t.Errorf("default is %q, want telegram", got.Default)
	}
	if got.Key != c.Key || len(got.Backends) != 2 {
		t.Errorf("changing the default changed the key or the places: %+v", got)
	}
	if err := setDefault("telegram"); err != nil {
		t.Errorf("setting the same default again failed: %v", err)
	}
	if err := setDefault("nowhere"); err == nil || !strings.Contains(err.Error(), "only r2, telegram") {
		t.Errorf("an unknown place gave %v, want the known places", err)
	}
	if err := run([]string{"default", "r2"}); err != nil {
		t.Errorf("the command gave %v", err)
	}
	got, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Default != "r2" {
		t.Errorf("the command left the default at %q", got.Default)
	}
	if err := run([]string{"default"}); err != errUsage {
		t.Errorf("a default without a place gave %v, want the usage", err)
	}
}

func TestSetDefaultWithoutConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("TAAS_CONFIG", path)
	if err := setDefault("telegram"); err == nil || !strings.Contains(err.Error(), "no places") {
		t.Errorf("a settings file that does not exist gave %v", err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := setDefault("telegram"); err == nil || !strings.Contains(err.Error(), "no places") {
		t.Errorf("empty settings gave %v", err)
	}
}
