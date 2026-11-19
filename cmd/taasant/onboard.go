package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vedantlavale/taasant/tgstore"
)

// The questions of the first start, in the order they are asked.
type step int

const (
	askBackend step = iota // Telegram, R2, S3 or MinIO?
	askToken
	askPlace // a channel, or the chat with the bot?
	findChat
	askField   // one question about an S3 bucket
	checkField // is the bucket usable with what was typed?
	askDefault // only when there already was a place: should this one be the default?
	ready
)

// A provider is a choice on the first screen. The S3 ones are all the same
// store, they only ask different questions and build the target from the
// answers.
type provider struct {
	name, label, about string
	fields             []field
	build              func(answers []string) target
}

type field struct {
	label, hint string
	secret      bool
	// problem, if not nil, says what is wrong with an answer.
	problem func(answer string) string
}

func isURL(answer string) string {
	if !strings.HasPrefix(answer, "http://") && !strings.HasPrefix(answer, "https://") {
		return "The address must start with http:// or https://"
	}
	return ""
}

// isBucket catches a name no bucket has, before it is tried.
func isBucket(answer string) string {
	if !bucketName.MatchString(answer) {
		return "A bucket name is 3 to 63 lowercase letters, numbers, dots and dashes."
	}
	return ""
}

// isAccountID catches what people paste by mistake, such as the whole
// address of the bucket.
func isAccountID(answer string) string {
	if raw, err := hex.DecodeString(answer); err != nil || len(raw) != 16 {
		return "An Account ID is 32 letters and numbers. Copy only that from the R2 page."
	}
	return ""
}

var providers = []provider{
	{name: "telegram", label: "Telegram", about: "free, through a bot of your own"},
	{
		name: "r2", label: "Cloudflare R2", about: "paid, no fee for downloads",
		fields: []field{
			{"Account ID", "Cloudflare dashboard, R2 Object Storage: it is on the right. For a bucket in the EU, choose the last option instead", false, isAccountID},
			{"Bucket name", "the bucket you made in R2", false, isBucket},
			{"Access Key ID", "R2, Manage API tokens: make a token that can read and write objects", false, nil},
			{"Secret Access Key", "shown once, together with the Access Key ID", true, nil},
		},
		build: func(a []string) target {
			return target{Type: "s3", Endpoint: "https://" + a[0] + ".r2.cloudflarestorage.com", Region: "auto", Bucket: a[1], AccessKey: a[2], SecretKey: a[3]}
		},
	},
	{
		name: "s3", label: "Amazon S3", about: "paid",
		fields: []field{
			{"Region", "where the bucket is, such as eu-west-1", false, nil},
			{"Bucket name", "the bucket you made in S3", false, isBucket},
			{"Access Key ID", "IAM user with read and write access to that bucket", false, nil},
			{"Secret Access Key", "shown once, together with the Access Key ID", true, nil},
		},
		build: func(a []string) target {
			return target{Type: "s3", Endpoint: "https://s3." + a[0] + ".amazonaws.com", Region: a[0], Bucket: a[1], AccessKey: a[2], SecretKey: a[3]}
		},
	},
	{
		name: "minio", label: "MinIO or other S3", about: "your own server, or any service with an S3 address",
		fields: []field{
			{"Server address", "such as http://localhost:9000", false, isURL},
			{"Bucket name", "the bucket you made on the server", false, isBucket},
			{"Access Key", "the user name of the server", false, nil},
			{"Secret Key", "the password of the server", true, nil},
		},
		build: func(a []string) target {
			return target{Type: "s3", Endpoint: a[0], Region: "us-east-1", Bucket: a[1], AccessKey: a[2], SecretKey: a[3]}
		},
	},
}

// onboard is the model of the first start. It fills in conf one question at
// a time and saves it at the end.
type onboard struct {
	step    step
	choice  int
	input   string // what has been typed or pasted so far
	notice  string
	waiting bool // Telegram is checking the token

	conf     config
	provider int      // the place chosen on the first screen
	answers  []string // the answers about a bucket, up to the question being asked
	checking bool     // a bucket is being tried
	again    bool     // setup was run for a place to add, not for the first start
	token    string   // the bot token, until it is saved with the chat
	chatID   int64
	place    string // what the place that was set up is called
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
	bucketChecked struct{ err error }
	chatFound     struct {
		search int
		chat   tgstore.Chat
		heard  int
		err    error
	}
)

// setUp asks the questions if the settings are not complete yet. It reports
// whether the menu can open: only once there is a place for the files and a key.
func setUp() (bool, error) {
	if _, err := settings(); err == nil {
		return true, nil
	}
	return askQuestions(false)
}

// askQuestions runs the setup screens. again says the settings were complete
// and a place is being added.
func askQuestions(again bool) (bool, error) {
	path, err := configPath()
	if err != nil {
		return false, err
	}
	conf, err := startFrom(path)
	if err != nil {
		return false, err
	}
	end, err := tea.NewProgram(onboard{path: path, conf: conf, heard: -1, again: again}, tea.WithAltScreen()).Run()
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
	case bucketChecked:
		o.checking = false
		if msg.err != nil {
			o.notice = "That did not work: " + msg.err.Error()
			return o, nil
		}
		return o.finish()
	case chatFound:
		switch {
		case msg.search != o.search:
		case msg.err != nil:
			o.notice, o.step = msg.err.Error(), askPlace
		default:
			o.chatID, o.heard = msg.chat.ID, msg.heard
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
	case askBackend:
		o.choice = move(o.choice, key, len(providers))
		switch key {
		case "enter":
			o.provider, o.choice, o.input = o.choice, 0, ""
			o.answers = nil
			o.step = askToken
			if len(providers[o.provider].fields) > 0 {
				o.step = askField
			}
		case "esc":
			return o, tea.Quit
		}

	case askToken:
		switch {
		case o.waiting:
		case key == "esc":
			o.step, o.choice, o.input = askBackend, o.provider, ""
		case key == "enter" && strings.TrimSpace(o.input) != "":
			// Another bot hears of other updates, so the search starts over.
			o.token, o.heard, o.waiting = strings.TrimSpace(o.input), -1, true
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
			return o.finish()
		}

	case askField:
		fields := providers[o.provider].fields
		answer := strings.TrimSpace(o.input)
		switch {
		case key == "esc":
			o.back()
		case key == "enter" && answer != "":
			if problem := fields[len(o.answers)].problem; problem != nil && problem(answer) != "" {
				o.notice = problem(answer)
				break
			}
			o.answers, o.input = append(o.answers, answer), ""
			if len(o.answers) == len(fields) {
				o.step, o.checking = checkField, true
				return o, o.checkBucket()
			}
		default:
			o.input = typed(o.input, msg)
		}

	case checkField:
		switch {
		case o.checking:
		case key == "esc":
			o.back()
		case key == "enter":
			o.checking = true
			return o, o.checkBucket()
		}

	case askDefault:
		o.choice = move(o.choice, key, 2)
		if key == "enter" {
			if o.choice == 0 {
				o.conf.Default = o.place
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

// back goes to the question before the one being asked, with its answer
// typed in again, or to the first screen from the first question.
func (o *onboard) back() {
	if len(o.answers) == 0 {
		o.step, o.choice, o.input = askBackend, o.provider, ""
		return
	}
	o.step = askField
	o.input, o.answers = o.answers[len(o.answers)-1], o.answers[:len(o.answers)-1]
}

// checkBucket tries the bucket with what was typed.
func (o onboard) checkBucket() tea.Cmd {
	t := providers[o.provider].build(o.answers)
	return func() tea.Msg {
		store, err := open(t)
		if err != nil {
			return bucketChecked{err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return bucketChecked{store.(interface{ Check(context.Context) error }).Check(ctx)}
	}
}

func (o onboard) target() target {
	if p := providers[o.provider]; len(p.fields) > 0 {
		return p.build(o.answers)
	}
	return target{Type: "telegram", Token: o.token, ChatID: o.chatID}
}

// finish adds the place that was set up. It becomes the default, unless
// there already was one and the person says to keep it.
func (o onboard) finish() (tea.Model, tea.Cmd) {
	name, t := providers[o.provider].name, o.target()
	// Setting up a place that exists already replaces it, which is how keys
	// are fixed. But a different bucket or bot is another place: files are
	// stored in the old one, and would become unreachable.
	if old, found := o.conf.Backends[name]; found && !old.sameAs(t) {
		name = freeName(o.conf.Backends, name)
	}
	o.place = name
	o.conf.set(name, t)
	if o.again && o.conf.Default != name {
		o.step, o.choice = askDefault, 0
		return o, nil
	}
	o.conf.Default = name
	return o.save()
}

func (o onboard) store() *tgstore.Store {
	store := tgstore.New(o.token, 0)
	if o.endpoint != "" {
		store.Endpoint = o.endpoint
	}
	return store
}

func (o onboard) save() (tea.Model, tea.Cmd) {
	if o.conf.Key == "" {
		secret := make([]byte, 32)
		rand.Read(secret)
		o.conf.Key = hex.EncodeToString(secret)
	}
	if err := saveConfig(o.path, o.conf); err != nil {
		o.notice = err.Error()
		return o, nil
	}
	o.step = ready
	return o, nil
}

// position says how far along the questions are: step i of n. The first
// screen has no number, as how many questions follow depends on its answer.
func (o onboard) position() (i, n int) {
	switch o.step {
	case askToken, askPlace, findChat:
		return int(o.step-askToken) + 1, 3
	case askField, checkField:
		n = len(providers[o.provider].fields)
		return min(len(o.answers)+1, n), n
	}
	return 0, 0
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
	case askBackend:
		lines = []string{"Where should taasant keep your files?", ""}
		for i, p := range providers {
			lines = append(lines, option(i, p.label, p.about))
		}
		lines = append(lines, "", dimStyle.Render("You can add more places later with: taasant setup"))
		keys = "↑↓  |  Enter  |  Esc Quit"
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
	case askField:
		p := providers[o.provider]
		f := p.fields[len(o.answers)]
		shown := field
		if f.secret {
			shown = "  > " + strings.Repeat("•", min(len([]rune(o.input)), 40)) + cursorStyle.Render("█")
		}
		lines = []string{dimStyle.Render(p.label), "", f.label, dimStyle.Render(f.hint), "", shown}
	case checkField:
		lines = []string{dimStyle.Render(providers[o.provider].label), "", "Trying the bucket with a small test file…"}
		if !o.checking {
			lines = []string{dimStyle.Render(providers[o.provider].label), "", "The bucket could not be used."}
			keys = "Enter Try again  |  Esc Back"
		}
	case askDefault:
		name := providers[o.provider].label
		lines = []string{
			goodStyle.Render("✓ " + name + " works."),
			"",
			"Should new files go there unless you say otherwise?",
			"",
			option(0, "Yes", "use "+name+" from now on"),
			option(1, "No", "keep "+o.conf.Default+", and pick "+o.place+" with --to"),
		}
		keys = "↑↓  |  Enter"
	case ready:
		lines = []string{
			goodStyle.Render("✓ All set."),
			"",
			"New files go to " + o.conf.Default + ". Your settings and the key to your files are saved in:",
			"  " + o.path,
			"",
			warnStyle.Render("Without the key nobody can read your files, you included."),
			warnStyle.Render("Put a copy of this file in your password manager now."),
		}
		if len(o.conf.Backends) > 1 {
			lines = append(lines[:4], append([]string{"", dimStyle.Render("To use another place for one file: taasant upload --to <place> <file>")}, lines[4:]...)...)
		}
		keys = "Enter Open the menu"
		if o.again {
			keys = "Enter Done"
		}
	}

	title := titleStyle.Render("Set up taasant")
	if i, n := o.position(); n > 0 {
		title += dimStyle.Render(fmt.Sprintf("  step %d of %d", i, n))
	}
	lines = append([]string{"", title, ""}, lines...)
	return strings.Join(append(lines, badStyle.Render(o.notice), "", dimStyle.Render(keys)), "\n")
}
