package main

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vedantlavale/taasant/tgstore"
)

var menuItems = []struct{ name, about, action string }{
	{"Upload", "Browse your folders and send files", "upload"},
	{"Files", "See what you have stored", ""},
	{"Download", "Get a file back", "download"},
	{"Delete", "Remove a file", "delete"},
}

type screen int

const (
	home screen = iota
	browsing
	listing
	working
	helping
)

// A job is one upload, download or delete, and goes through these states.
const (
	waiting = iota
	running
	succeeded
	failed
)

type job struct {
	action string // "upload", "download" or "delete"
	name   string
	size   int64
	path   string // upload: the file to send. download: the file to write
	entry  entry  // download and delete: the stored file
	state  int
	err    error
}

type item struct {
	name string
	dir  bool
	size int64
}

// model is everything the menu knows. Bubble Tea passes it to Update for
// every key press and to View to draw the screen.
type model struct {
	screen        screen
	width, height int
	notice        string // a problem to show on the current screen

	store     *tgstore.Store
	key       []byte
	setup     error // why store and key are missing, if they are
	indexFile string
	entries   []entry // the stored files, newest first

	choice int // row of the start menu
	cursor int // row of the list on the other screens

	// Browsing folders for files to upload.
	startDir string // where taasant was started, downloads are saved here
	dir      string
	items    []item
	marked   map[string]int64 // the files picked so far: path and size

	// The list of stored files.
	action  string // what Enter does there: "download", "delete" or nothing
	confirm bool   // waiting for Y before a delete

	// Jobs run one after another while the screen keeps drawing.
	jobs     []job
	current  int
	busy     bool
	changed  bool          // the index changed, so it needs a new backup
	quitting bool          // leave as soon as the running job has stopped
	moved    *atomic.Int64 // bytes the running job has sent or received
	ctx      context.Context
	cancel   context.CancelFunc
	frame    int // which picture of the spinner to show
}

// These are the messages that come back from work done in the background.
type (
	tickMsg  struct{}
	finished struct{ err error } // the running job ended
	backedUp struct{ err error } // the index was sent to Telegram
)

// menu runs the full screen program you get from plain "taasant".
func menu() error {
	if done, err := setUp(); !done {
		return err
	}
	m, err := newModel()
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel() (model, error) {
	indexFile, err := indexPath()
	if err != nil {
		return model{}, err
	}
	entries, err := loadNewest(indexFile)
	if err != nil {
		return model{}, err
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = string(filepath.Separator)
	}
	m := model{width: 80, height: 24, indexFile: indexFile, entries: entries, startDir: dir, dir: dir, marked: map[string]int64{}}
	m.store, m.key, m.setup = connect()
	return m, nil
}

func loadNewest(indexFile string) ([]entry, error) {
	entries, err := loadIndex(indexFile)
	slices.SortFunc(entries, func(a, b entry) int { return b.Uploaded.Compare(a.Uploaded) })
	return entries, err
}

func (m model) Init() tea.Cmd { return nil }

// Update is called with everything that happens: a key press, a new window
// size, or a message from a job. It returns the model as it is afterwards,
// and optionally more work to do in the background.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.pressed(strings.ToLower(msg.String()))
	case tickMsg:
		if m.busy {
			m.frame++
			return m, tick()
		}
	case finished:
		return m.jobEnded(msg.err)
	case backedUp:
		if msg.err != nil {
			m.notice = "The list of files was not backed up: " + msg.err.Error()
		}
		return m.rest()
	}
	return m, nil
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m model) pressed(key string) (model, tea.Cmd) {
	if key == "ctrl+c" || (key == "q" && !m.confirm) {
		if !m.busy {
			return m, tea.Quit
		}
		// Stop the transfer first, so it can clean up after itself. The
		// program ends when the job reports back.
		m.quitting = true
		m.cancel()
		return m, nil
	}
	m.notice = ""

	switch m.screen {
	case home:
		return m.homeKey(key)
	case browsing:
		return m.browseKey(key)
	case listing:
		return m.listKey(key)
	case working:
		if m.busy && key == "esc" {
			m.cancel()
		} else if !m.busy && (key == "enter" || key == "esc") {
			m.screen = home
		}
	case helping:
		m.screen = home
	}
	return m, nil
}

// move returns where the cursor is after key, kept inside a list of count rows.
func move(cursor int, key string, count int) int {
	switch key {
	case "up", "k":
		cursor--
	case "down", "j":
		cursor++
	}
	return max(0, min(cursor, count-1))
}

func (m model) homeKey(key string) (model, tea.Cmd) {
	m.choice = move(m.choice, key, len(menuItems))
	switch key {
	case "enter":
	case "1", "2", "3", "4":
		m.choice = int(key[0] - '1')
	case "h":
		m.screen = helping
		return m, nil
	case "esc":
		return m, tea.Quit
	default:
		return m, nil
	}

	m.cursor, m.action, m.screen = 0, menuItems[m.choice].action, listing
	if m.action == "upload" {
		m.screen = browsing
		m.enter(m.dir)
	}
	return m, nil
}

func (m model) browseKey(key string) (model, tea.Cmd) {
	m.cursor = move(m.cursor, key, len(m.items))
	var here item
	if m.cursor < len(m.items) {
		here = m.items[m.cursor]
	}
	path := filepath.Join(m.dir, here.name)
	isFile := here.name != "" && !here.dir

	switch key {
	case " ":
		if _, found := m.marked[path]; found {
			delete(m.marked, path)
		} else if isFile {
			m.marked[path] = here.size
		}
	case "right", "l":
		if here.dir {
			m.enter(path)
		}
	case "left", "backspace":
		m.enter(filepath.Dir(m.dir))
	case "enter":
		switch {
		case len(m.marked) > 0:
			var jobs []job
			for _, path := range slices.Sorted(maps.Keys(m.marked)) {
				jobs = append(jobs, job{action: "upload", name: filepath.Base(path), size: m.marked[path], path: path})
			}
			return m.start(jobs)
		case here.dir:
			m.enter(path)
		case isFile:
			return m.start([]job{{action: "upload", name: here.name, size: here.size, path: path}})
		}
	case "esc":
		m.screen = home
	}
	return m, nil
}

// enter moves the browser to dir, or says why it could not.
func (m *model) enter(dir string) {
	items, err := listDir(dir)
	if err != nil {
		m.notice = err.Error()
		return
	}
	// Going up puts the cursor back on the folder we came out of.
	came := filepath.Base(m.dir)
	m.cursor = max(0, slices.IndexFunc(items, func(it item) bool { return it.name == came }))
	m.dir, m.items = dir, items
}

// listDir returns the folders in dir and then the files, each in alphabetical
// order. Hidden files, the ones that start with a dot, are left out.
func listDir(dir string) ([]item, error) {
	found, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []item
	for _, f := range found {
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
		// os.Stat follows links, so a link to a folder counts as a folder.
		info, err := os.Stat(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		if info.IsDir() || info.Mode().IsRegular() {
			items = append(items, item{f.Name(), info.IsDir(), info.Size()})
		}
	}
	slices.SortFunc(items, func(a, b item) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	return items, nil
}

func (m model) listKey(key string) (model, tea.Cmd) {
	if m.cursor >= len(m.entries) {
		if key == "esc" || key == "left" {
			m.screen = home
		}
		return m, nil
	}
	e := m.entries[m.cursor]
	if m.confirm {
		m.confirm = false
		if key == "y" {
			return m.start([]job{{action: "delete", name: e.Name, size: e.Size, entry: e}})
		}
		return m, nil
	}

	m.cursor = move(m.cursor, key, len(m.entries))
	switch key {
	case "enter":
		if m.action == "download" {
			return m.start([]job{{action: "download", name: e.Name, size: e.Size, entry: e, path: filepath.Join(m.startDir, e.Name)}})
		}
		m.confirm = m.action == "delete"
	case "esc", "left":
		m.screen = home
	}
	return m, nil
}

// start queues the jobs and begins the first one.
func (m model) start(jobs []job) (model, tea.Cmd) {
	if m.setup != nil {
		m.notice = "Not set up yet: " + m.setup.Error()
		return m, nil
	}
	m.screen, m.jobs, m.current = working, jobs, 0
	m.busy, m.changed = true, false
	m.ctx, m.cancel = context.WithCancel(context.Background())
	clear(m.marked)
	m, work := m.begin()
	return m, tea.Batch(work, tick())
}

// begin starts the job at m.current. The function it returns is run by
// Bubble Tea in a goroutine, and what that returns comes back to Update.
func (m model) begin() (model, tea.Cmd) {
	m.jobs[m.current].state = running
	m.moved = new(atomic.Int64)
	j, moved := m.jobs[m.current], m.moved

	return m, func() tea.Msg {
		// This runs while the screen is being drawn, so it only leaves a
		// number behind for View to read.
		report := func(done, total int64) { moved.Store(done) }
		var err error
		switch j.action {
		case "upload":
			_, err = addFile(m.ctx, m.store, m.key, m.indexFile, j.path, report)
		case "download":
			err = download(m.ctx, m.store, m.key, j.entry, j.path, report)
		case "delete":
			err = removeFile(m.ctx, m.store, m.key, m.indexFile, j.entry)
		}
		return finished{err}
	}
}

// jobEnded writes down how the running job went and starts the next one.
// When none are left, a changed index is backed up.
func (m model) jobEnded(err error) (model, tea.Cmd) {
	j := &m.jobs[m.current]
	j.state, j.err = succeeded, err
	if err != nil {
		j.state = failed
	} else if j.action != "download" {
		m.changed = true
	}
	if entries, err := loadNewest(m.indexFile); err == nil {
		m.entries = entries
	}

	for m.current+1 < len(m.jobs) {
		m.current++
		if m.ctx.Err() == nil {
			return m.begin()
		}
		m.jobs[m.current].state, m.jobs[m.current].err = failed, m.ctx.Err()
	}
	if m.changed && !m.quitting {
		m.changed = false
		return m, func() tea.Msg {
			return backedUp{backupIndex(context.Background(), m.store, m.key, m.indexFile)}
		}
	}
	return m.rest()
}

// rest ends the work. The screen stays, so the results can be read.
func (m model) rest() (model, tea.Cmd) {
	m.busy = false
	m.cancel()
	if m.quitting {
		return m, tea.Quit
	}
	return m, nil
}
