package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// answer types keys into the first start and runs the questions it sends to
// Telegram, feeding the answers back in.
func answer(o onboard, keys ...string) onboard {
	for _, key := range keys {
		next, cmd := o.Update(keyMsg(key))
		o = next.(onboard)
		if cmd == nil {
			continue
		}
		if msg := cmd(); msg != (tea.QuitMsg{}) {
			next, _ = o.Update(msg)
			o = next.(onboard)
		}
	}
	return o
}

// fakeBot is a Telegram that knows one bot and one channel. It writes down
// the offset of every search for a chat.
func fakeBot(t *testing.T, offsets *[]string) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botTOKEN/getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"username":"my_bot"}}`)
		case "/botTOKEN/getUpdates":
			*offsets = append(*offsets, r.FormValue("offset"))
			fmt.Fprint(w, `{"ok":true,"result":[{"update_id":1,"channel_post":{"chat":{"id":-100,"type":"channel","title":"my files"}}}]}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"ok":false,"description":"Unauthorized"}`)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestOnboardNewStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taas", "config.json")
	offsets := &[]string{}
	o := onboard{path: path, heard: -1, endpoint: fakeBot(t, offsets)}

	if !strings.Contains(o.View(), "step 1 of 3") {
		t.Fatalf("the first start does not open on the token:\n%s", o.View())
	}
	o = answer(o, "WRONG", "enter")
	if o.step != askToken || !strings.Contains(o.View(), "That token does not work") {
		t.Fatalf("a wrong token was not refused:\n%s", o.View())
	}
	o = answer(o, "backspace", "backspace", "backspace", "backspace", "backspace", "TOKEN", "enter")
	if o.step != askPlace || !strings.Contains(o.View(), "Connected to @my_bot") {
		t.Fatalf("the token was not accepted:\n%s", o.View())
	}
	o = answer(o, "enter")
	if !strings.Contains(o.View(), "Found my files") {
		t.Fatalf("the channel was not found:\n%s", o.View())
	}
	// "No, look again" must wait for something newer than what was found.
	o = answer(o, "esc", "enter")
	if got := strings.Join(*offsets, " "); got != "0 2" {
		t.Errorf("searched from offsets %q, want 0 then 2", got)
	}
	o = answer(o, "enter")
	if o.step != ready {
		t.Fatalf("not finished:\n%s", o.View())
	}

	saved, err := loadConfig(path)
	if err != nil || saved.Token != "TOKEN" || saved.ChatID != -100 || saved.check() != nil {
		t.Errorf("saved %+v, %v", saved, err)
	}
}

func TestSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("TAAS_CONFIG", path)
	for _, name := range []string{"TG_BOT_TOKEN", "TG_CHAT_ID", "TAAS_KEY"} {
		t.Setenv(name, "")
	}
	if _, err := settings(); err == nil {
		t.Error("settings were accepted with nothing set")
	}

	first := config{Token: "TOKEN", ChatID: -100, Key: strings.Repeat("ab", 32)}
	if err := saveConfig(path, first); err != nil {
		t.Fatal(err)
	}
	if got, err := settings(); err != nil || got != first {
		t.Errorf("settings = %+v, %v", got, err)
	}

	t.Setenv("TG_CHAT_ID", "42")
	if got, _ := settings(); got.ChatID != 42 || got.Token != "TOKEN" {
		t.Errorf("the environment did not win: %+v", got)
	}
}

// A key that is saved already must survive the first start, since files
// may have been stored with it.
func TestOnboardKeepsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := strings.Repeat("ab", 32)
	o := onboard{path: path, conf: config{Key: old}, heard: -1, endpoint: fakeBot(t, &[]string{})}

	o = answer(o, "TOKEN", "enter", "enter", "enter")
	if saved, err := loadConfig(path); o.step != ready || err != nil || saved.Key != old {
		t.Errorf("step %d, saved %+v, %v", o.step, saved, err)
	}
}

// The first start must not make a new key while files stored with an older
// one are listed in the index.
func TestStartFrom(t *testing.T) {
	dir := t.TempDir()
	path, indexFile := filepath.Join(dir, "config.json"), filepath.Join(dir, "index.json")
	t.Setenv("TAAS_INDEX", indexFile)
	t.Setenv("TAAS_KEY", "")

	if conf, err := startFrom(path); err != nil || conf != (config{}) {
		t.Errorf("a new computer: %+v, %v", conf, err)
	}
	if err := saveIndex(indexFile, []entry{{Name: "holiday.jpg", ID: "1:abc"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := startFrom(path); err == nil {
		t.Error("the first start would make a new key although a file is stored")
	}
	old := strings.Repeat("ab", 32)
	t.Setenv("TAAS_KEY", old)
	if conf, err := startFrom(path); err != nil || conf.Key != old {
		t.Errorf("the key in TAAS_KEY was not kept: %+v, %v", conf, err)
	}
}
