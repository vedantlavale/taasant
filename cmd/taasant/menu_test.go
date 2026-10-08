package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vedantlavale/taasant/tgstore"
)

// fakeTelegram keeps the files it is sent in memory.
type fakeTelegram struct {
	mu    sync.Mutex
	files map[string][]byte
	sent  int
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.URL.Path {
	case "/botTOKEN/sendDocument":
		file, _, err := r.FormFile("document")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(file)
		f.sent++
		id := fmt.Sprintf("file%d", f.sent)
		f.files[id] = data
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"document":{"file_id":%q}}}`, f.sent, id)
	case "/botTOKEN/deleteMessage":
		delete(f.files, "file"+r.FormValue("message_id"))
		fmt.Fprint(w, `{"ok":true,"result":true}`)
	case "/botTOKEN/getFile":
		fmt.Fprintf(w, `{"ok":true,"result":{"file_path":%q}}`, "documents/"+r.FormValue("file_id"))
	default:
		var id string
		fmt.Sscanf(r.URL.Path, "/file/botTOKEN/documents/%s", &id)
		w.Write(f.files[id])
	}
}

// testModel is a menu that talks to a fake Telegram and starts in a folder
// holding notes.txt and photos/a.jpg.
func testModel(t *testing.T) (model, *fakeTelegram) {
	fake := &fakeTelegram{files: map[string][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	store := tgstore.New("TOKEN", 1)
	store.Endpoint = server.URL

	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "photos"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "photos", "a.jpg"), []byte("picture"), 0o644)

	return model{
		width: 80, height: 24,
		store: store, key: make([]byte, 32),
		indexFile: filepath.Join(t.TempDir(), "index.json"),
		startDir:  dir, dir: dir, marked: map[string]int64{},
	}, fake
}

// press types keys into the model. After each one it runs the background
// work Update asked for and feeds the results back in, as Bubble Tea does.
func press(m model, keys ...string) model {
	for _, key := range keys {
		next, cmd := m.Update(keyMsg(key))
		m = next.(model)
		for queue := []tea.Cmd{cmd}; len(queue) > 0; queue = queue[1:] {
			if queue[0] == nil {
				continue
			}
			switch msg := queue[0]().(type) {
			case tea.BatchMsg:
				queue = append(queue, msg...)
			case tickMsg, tea.QuitMsg:
			default:
				next, cmd := m.Update(msg)
				m = next.(model)
				queue = append(queue, cmd)
			}
		}
	}
	return m
}

func keyMsg(key string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, " ": tea.KeySpace, "backspace": tea.KeyBackspace,
	}
	if keyType, found := named[key]; found {
		return tea.KeyMsg{Type: keyType}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func TestUploadDownloadDelete(t *testing.T) {
	m, fake := testModel(t)

	// Mark notes.txt, go into photos, mark a.jpg, come back out.
	m = press(m, "1", "down", " ", "up", "right", " ", "left")
	if len(m.marked) != 2 || m.items[m.cursor].name != "photos" {
		t.Fatalf("marked %v with the cursor on %q, want 2 files and photos", m.marked, m.items[m.cursor].name)
	}
	if view := m.View(); !strings.Contains(view, "2 selected, 12 B") || !strings.Contains(view, "Enter Upload 2") {
		t.Errorf("the browser does not show the selection:\n%s", view)
	}

	m = press(m, "enter")
	if m.screen != working || m.busy || len(m.jobs) != 2 || m.jobs[0].state != succeeded || m.jobs[1].state != succeeded {
		t.Fatalf("after uploading: screen %d, busy %v, jobs %+v", m.screen, m.busy, m.jobs)
	}
	if view := m.View(); !strings.Contains(view, "2 done") || !strings.Contains(view, "✓ notes.txt") {
		t.Errorf("the results are not shown:\n%s", view)
	}
	saved, _ := loadIndex(m.indexFile)
	if len(saved) != 2 || len(m.entries) != 2 || len(m.marked) != 0 {
		t.Errorf("index has %d entries, the menu %d, %d still marked; want 2, 2, 0", len(saved), len(m.entries), len(m.marked))
	}
	if _, err := os.Stat(m.indexFile + ".remote"); err != nil {
		t.Errorf("the index was not backed up: %v", err)
	}

	// notes.txt is still in the start folder, and a download never overwrites.
	wanted := slices.IndexFunc(m.entries, func(e entry) bool { return e.Name == "notes.txt" })
	m = press(m, "enter", "3")
	m.cursor = wanted
	m = press(m, "enter")
	if m.jobs[0].state != failed || !strings.Contains(m.View(), "1 failed") {
		t.Errorf("downloading over an existing file: %+v", m.jobs[0])
	}

	m.startDir = t.TempDir()
	m = press(m, "enter", "3")
	m.cursor = wanted
	m = press(m, "enter")
	if data, err := os.ReadFile(filepath.Join(m.startDir, "notes.txt")); string(data) != "hello" {
		t.Errorf("downloaded %q, %v; want hello", data, err)
	}

	// Delete asks first: anything but Y leaves the file alone.
	stored := len(fake.files)
	m = press(m, "enter", "4", "enter")
	if !m.confirm || !strings.Contains(m.View(), "Delete "+m.entries[0].Name+"?") {
		t.Fatalf("delete did not ask first:\n%s", m.View())
	}
	m = press(m, "n")
	if m.confirm || m.screen != listing || len(fake.files) != stored {
		t.Fatal("answering N still deleted something")
	}
	m = press(m, "enter", "y")
	if len(m.entries) != 1 || m.jobs[0].state != succeeded {
		t.Errorf("after deleting: %d entries, job %+v", len(m.entries), m.jobs[0])
	}
}

func TestNotSetUp(t *testing.T) {
	m, fake := testModel(t)
	m.store, m.setup = nil, errors.New("TG_BOT_TOKEN is not set")

	m = press(m, "1", "down", "enter")
	if m.screen != browsing || !strings.Contains(m.View(), "Not set up yet: TG_BOT_TOKEN is not set") {
		t.Errorf("screen %d, want the browser with the reason:\n%s", m.screen, m.View())
	}
	if fake.sent != 0 {
		t.Error("something was uploaded anyway")
	}
}

// Quitting in the middle of a job must wait for the job, or a half written
// download would be left behind.
func TestQuitWhileBusy(t *testing.T) {
	m, _ := testModel(t)
	m = press(m, "1", "down")

	next, cmd := m.Update(keyMsg("enter"))
	work := cmd().(tea.BatchMsg)[0]
	next, cmd = next.Update(keyMsg("q"))
	if cmd != nil || !next.(model).quitting {
		t.Fatal("Q during a job should wait for the job")
	}

	next, cmd = next.Update(work())
	m = next.(model)
	if m.jobs[0].state != failed || !strings.Contains(m.View(), "cancelled") {
		t.Errorf("the job was not cancelled: %+v", m.jobs[0])
	}
	if cmd == nil {
		t.Fatal("the program did not quit once the job had stopped")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("the program did not quit once the job had stopped")
	}
}

func TestScreens(t *testing.T) {
	m, _ := testModel(t)
	if view := m.View(); !strings.Contains(view, "➤ 1. Upload") || !strings.Contains(view, "0 files, 0 B stored") || !strings.Contains(view, logo[0]) {
		t.Errorf("start screen:\n%s", view)
	}
	// Too narrow for the logo: the name is written out instead.
	narrow := m
	narrow.width = 50
	if view := narrow.View(); strings.Contains(view, logo[0]) || !strings.Contains(view, "taasant") {
		t.Errorf("start screen, 50 columns wide:\n%s", view)
	}
	m = press(m, "down")
	if !strings.Contains(m.View(), "➤ 2. Files") {
		t.Errorf("↓ did not move the cursor:\n%s", m.View())
	}
	m = press(m, "enter")
	if view := m.View(); !strings.Contains(view, "nothing here") || !strings.Contains(view, "NAME") {
		t.Errorf("empty list of files:\n%s", view)
	}
	m = press(m, "esc", "h")
	if !strings.Contains(m.View(), "TG_BOT_TOKEN") {
		t.Errorf("help screen:\n%s", m.View())
	}
	if m = press(m, "x"); m.screen != home {
		t.Error("a key did not close the help screen")
	}

	// A folder that cannot be opened leaves you where you were, with the reason.
	m = press(m, "1")
	m.enter(filepath.Join(m.dir, "missing"))
	if m.dir != m.startDir || m.notice == "" {
		t.Errorf("entering a missing folder: dir %q, notice %q", m.dir, m.notice)
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "zoo"), 0o755)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("12345"), 0o644)
	os.WriteFile(filepath.Join(dir, "A.txt"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o644)
	want := []item{{"zoo", true, 0}, {"A.txt", false, 0}, {"b.txt", false, 5}}
	if os.Symlink(filepath.Join(dir, "zoo"), filepath.Join(dir, "link")) == nil {
		want = slices.Insert(want, 0, item{"link", true, 0})
	}

	got, err := listDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		if got[i].dir {
			got[i].size = 0
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("listDir = %v, want %v", got, want)
	}
}

func TestWindow(t *testing.T) {
	tests := []struct{ cursor, count, visible, first, last int }{
		{0, 3, 5, 0, 3},
		{0, 20, 5, 0, 5},
		{2, 20, 5, 0, 5},
		{10, 20, 5, 8, 13},
		{19, 20, 5, 15, 20},
		{0, 0, 5, 0, 0},
	}
	for _, test := range tests {
		first, last := window(test.cursor, test.count, test.visible)
		if first != test.first || last != test.last {
			t.Errorf("window(%d, %d, %d) = %d, %d; want %d, %d", test.cursor, test.count, test.visible, first, last, test.first, test.last)
		}
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		text  string
		width int
		want  string
	}{
		{"a.txt", 8, "a.txt   "},
		{"abcdefghij", 6, "abcde…"},
		{"世界.txt", 10, "世界.txt  "},
	}
	for _, test := range tests {
		if got := fit(test.text, test.width); got != test.want {
			t.Errorf("fit(%q, %d) = %q, want %q", test.text, test.width, got, test.want)
		}
	}
}

func TestShort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		dir   string
		limit int
		want  string
	}{
		{home, 40, "~"},
		{filepath.Join(home, "photos", "trips"), 40, "~/photos/trips"},
		{home + "-other", 200, home + "-other"},
		{"/var/log/very/deep/folder", 12, "…deep/folder"},
	}
	for _, test := range tests {
		if got := short(test.dir, test.limit); got != test.want {
			t.Errorf("short(%q, %d) = %q, want %q", test.dir, test.limit, got, test.want)
		}
	}
}
