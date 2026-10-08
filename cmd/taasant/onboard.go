package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vedantlavale/taasant/tgstore"
)

// The questions of the first start, in the order they are asked.
type step int

const (
	askToken step = iota
	askPlace      // a channel, or the chat with the bot?
	findChat
	ready
)

// onboard is the model of the first start. It fills in conf one question at
// a time and saves it at the end.
type onboard struct {
	step    step
	choice  int
	input   string // what has been typed or pasted so far
	notice  string
	waiting bool // Telegram is checking the token

	conf     config
	bot      string // the bot's username
	chat     string // the name of the chat that was found
	search   int    // counts the searches for a chat, to tell an old answer from a new one
	heard    int    // how far the searches have listened, see tgstore.FindChat
	path     string // where conf is saved
	endpoint string // tests point this at a fake Telegram
	cancel   context.CancelFunc
}

type (
	botChecked struct {
		name string
		err  error
	}
	chatFound struct {
		search int
		chat   tgstore.Chat
		heard  int
		err    error
	}
)

// setUp asks the questions if the settings are not complete yet. It reports
// whether the menu can open: only once there is a bot, a chat and a key.
func setUp() (bool, error) {
	if _, err := settings(); err == nil {
		return true, nil
	}
	path, err := configPath()
	if err != nil {
		return false, err
	}
	conf, err := startFrom(path)
	if err != nil {
		return false, err
	}
	end, err := tea.NewProgram(onboard{path: path, conf: conf, heard: -1}, tea.WithAltScreen()).Run()
	if err != nil || end.(onboard).step != ready {
		return false, err
	}
	_, err = settings()
	return err == nil, err
}

// startFrom returns the settings the first start begins with. A key that
// exists already is kept, in the file or in TAAS_KEY, because files may be
// stored with it. If files are stored and their key is nowhere to be found,
// making a new key would leave them unreadable, so that is refused.
func startFrom(path string) (config, error) {
	conf, err := loadConfig(path)
	if err != nil {
		return conf, fmt.Errorf("%s cannot be read: %w", path, err)
	}
	if conf.Key == "" {
		conf.Key = os.Getenv("TAAS_KEY")
	}
	if conf.Key != "" {
		return conf, nil
	}
	indexFile, err := indexPath()
	if err != nil {
		return conf, err
	}
	entries, err := loadIndex(indexFile)
	if err != nil {
		return conf, err
	}
	if len(entries) > 0 {
		return conf, fmt.Errorf("%d files were stored with a key this computer does not have, set TAAS_KEY to that key and run taasant again", len(entries))
	}
	return conf, nil
}

func (o onboard) Init() tea.Cmd { return nil }

func (o onboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return o.pressed(msg)
	case botChecked:
		o.waiting = false
		if msg.err != nil {
			// Telegram answers 401 or 404 to a token it does not know.
			o.notice = "That token could not be checked: " + msg.err.Error()
			if text := msg.err.Error(); strings.Contains(text, "401") || strings.Contains(text, "404") {
				o.notice = "That token does not work. Copy it again from @BotFather."
			}
			return o, nil
		}
		o.bot, o.step, o.choice = msg.name, askPlace, 0
	case chatFound:
		switch {
		case msg.search != o.search:
		case msg.err != nil:
			o.notice, o.step = msg.err.Error(), askPlace
		default:
			o.conf.ChatID, o.heard = msg.chat.ID, msg.heard
			o.chat = cmp.Or(msg.chat.Title, msg.chat.FirstName, "a chat")
		}
	}
	return o, nil
}

func (o onboard) pressed(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return o, tea.Quit
	}
	o.notice = ""

	switch o.step {
	case askToken:
		switch {
		case o.waiting:
		case key == "esc":
			return o, tea.Quit
		case key == "enter" && strings.TrimSpace(o.input) != "":
			o.conf.Token, o.waiting = strings.TrimSpace(o.input), true
			store := o.store()
			return o, func() tea.Msg {
				name, err := store.Me(context.Background())
				return botChecked{name, err}
			}
		default:
			o.input = typed(o.input, msg)
		}

	case askPlace:
		o.choice = move(o.choice, key, 2)
		switch key {
		case "enter":
			o.step, o.chat = findChat, ""
			o.search++
			ctx, cancel := context.WithCancel(context.Background())
			o.cancel = cancel
			store, private, search, heard := o.store(), o.choice == 1, o.search, o.heard
			return o, func() tea.Msg {
				chat, heard, err := store.FindChat(ctx, private, heard)
				return chatFound{search, chat, heard, err}
			}
		case "esc":
			o.step = askToken
		}

	case findChat:
		switch {
		case key == "esc":
			o.cancel()
			o.search++
			o.step = askPlace
		case key == "enter" && o.chat != "":
			if o.conf.Key == "" {
				secret := make([]byte, 32)
				rand.Read(secret)
				o.conf.Key = hex.EncodeToString(secret)
			}
			return o.save()
		}

	case ready:
		if key == "enter" {
			return o, tea.Quit
		}
	}
	return o, nil
}

// typed returns the text after a key press: letters and pasted text are
// added to the end, Backspace takes one letter away.
func typed(text string, msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyRunes:
		return text + string(msg.Runes)
	case tea.KeySpace:
		return text + " "
	case tea.KeyBackspace:
		if letters := []rune(text); len(letters) > 0 {
			return string(letters[:len(letters)-1])
		}
	}
	return text
}

func (o onboard) store() *tgstore.Store {
	store := tgstore.New(o.conf.Token, 0)
	if o.endpoint != "" {
		store.Endpoint = o.endpoint
	}
	return store
}

func (o onboard) save() (tea.Model, tea.Cmd) {
	if err := saveConfig(o.path, o.conf); err != nil {
		o.notice = err.Error()
		return o, nil
	}
	o.step = ready
	return o, nil
}

func (o onboard) View() string {
	option := func(i int, name, about string) string {
		if i == o.choice {
			return cursorStyle.Render("➤ "+name) + "  " + dimStyle.Render(about)
		}
		return "  " + name + "  " + dimStyle.Render(about)
	}
	// Only the end of a long token is shown, so the line never wraps.
	field := o.input
	if letters := []rune(field); len(letters) > 40 {
		field = "…" + string(letters[len(letters)-40:])
	}
	field = "  > " + field + cursorStyle.Render("█")

	var lines []string
	keys := "Enter  |  Esc Back"
	switch o.step {
	case askToken:
		lines = []string{
			"taasant keeps your files in Telegram through a bot of your own.",
			dimStyle.Render("Tip: a second Telegram account is a good home for this bot."),
			"",
			"1. In Telegram, open a chat with @BotFather.",
			"2. Send /newbot and answer its questions.",
			"3. Paste the token it gives you here.",
			"",
			field,
		}
		keys = "Enter  |  Esc Quit"
		if o.waiting {
			lines = append(lines, "", dimStyle.Render("Asking Telegram…"))
		}
	case askPlace:
		lines = []string{
			goodStyle.Render("✓ Connected to @" + o.bot),
			"",
			"Where should the bot keep your files?",
			"",
			option(0, "A private channel", "recommended: your files get a place of their own"),
			option(1, "The chat with the bot", "simpler, nothing else to create"),
		}
		keys = "↑↓  |  Enter  |  Esc Back"
	case findChat:
		lines = []string{
			"1. In Telegram, create a new private channel.",
			"2. Add @" + o.bot + " to it as an administrator, and leave its rights switched on.",
			"3. Post any message in the channel.",
		}
		if o.choice == 1 {
			lines = []string{"In Telegram, open a chat with @" + o.bot + " and press Start."}
		}
		lines = append(lines, "", dimStyle.Render("Waiting to hear from Telegram…"))
		keys = "Esc Back"
		if o.chat != "" {
			lines = []string{goodStyle.Render("✓ Found " + o.chat), "", "Is this the right place?"}
			keys = "Enter Yes, use it  |  Esc No, look again"
		}
	case ready:
		lines = []string{
			goodStyle.Render("✓ All set."),
			"",
			"Your bot, your chat and the key to your files are saved in:",
			"  " + o.path,
			"",
			warnStyle.Render("Without the key nobody can read your files, you included."),
			warnStyle.Render("Put a copy of this file in your password manager now."),
		}
		keys = "Enter Open the menu"
	}

	title := titleStyle.Render("Set up taasant")
	if o.step != ready {
		title += dimStyle.Render(fmt.Sprintf("  step %d of 3", o.step+1))
	}
	lines = append([]string{"", title, ""}, lines...)
	return strings.Join(append(lines, badStyle.Render(o.notice), "", dimStyle.Render(keys)), "\n")
}
