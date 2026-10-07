package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The name in block letters, 60 columns wide.
var logo = []string{
	"████████╗ █████╗  █████╗ ███████╗ █████╗ ███╗   ██╗████████╗",
	"╚══██╔══╝██╔══██╗██╔══██╗██╔════╝██╔══██╗████╗  ██║╚══██╔══╝",
	"   ██║   ███████║███████║███████╗███████║██╔██╗ ██║   ██║",
	"   ██║   ██╔══██║██╔══██║╚════██║██╔══██║██║╚██╗██║   ██║",
	"   ██║   ██║  ██║██║  ██║███████║██║  ██║██║ ╚████║   ██║",
	"   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═══╝   ╚═╝",
}

// The colours are the terminal's own 16, so they follow the user's theme.
var (
	plain       = lipgloss.NewStyle()
	dimStyle    = plain.Faint(true)
	badStyle    = plain.Foreground(lipgloss.Color("1"))
	goodStyle   = plain.Foreground(lipgloss.Color("2"))
	warnStyle   = plain.Foreground(lipgloss.Color("3"))
	linkStyle   = plain.Foreground(lipgloss.Color("4"))
	cursorStyle = plain.Foreground(lipgloss.Color("5"))
	folderStyle = linkStyle.Bold(true)
	titleStyle  = cursorStyle.Bold(true)
)

const spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

// View turns the model into the text of the screen. Bubble Tea calls it
// after every Update and works out which lines changed.
func (m model) View() string {
	switch m.screen {
	case browsing:
		return m.browseView()
	case listing:
		return m.listView()
	case working:
		return m.workView()
	case helping:
		return "\n" + usage + "\n\n" + dimStyle.Render("Any key Back  |  Q Quit")
	}
	return m.homeView()
}

// page puts a screen together: the heading, the rows that fit below it with
// the cursor's row among them, and the keys.
func (m model) page(heading []string, cursor, count int, row func(i int) string, keys string) string {
	lines := append([]string{""}, heading...)
	first, last := window(cursor, count, max(1, m.height-len(lines)-3))
	for i := first; i < last; i++ {
		lines = append(lines, row(i))
	}
	if count == 0 {
		lines = append(lines, dimStyle.Render("  nothing here"))
	}
	// A list too long to show says where the cursor is, above the keys.
	under := ""
	if last-first < count {
		under = dimStyle.Render(fmt.Sprintf("  %d of %d", cursor+1, count))
	}
	return strings.Join(append(lines, under, keys), "\n")
}

// window picks which rows to show when not all of them fit. The cursor stays
// in the middle until the list runs out.
func window(cursor, count, visible int) (first, last int) {
	first = max(0, min(cursor-visible/2, count-visible))
	return first, min(first+visible, count)
}

// fit cuts or pads text so that it fills exactly width columns.
func fit(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

// short writes a folder the way a shell prompt does, with ~ for the home
// folder. If it is still too long the start is cut off, since the end says
// where you are.
func short(dir string, limit int) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rest, found := strings.CutPrefix(dir, home); found && (rest == "" || rest[0] == filepath.Separator) {
			dir = "~" + rest
		}
	}
	if letters := []rune(dir); len(letters) > limit {
		return "…" + string(letters[len(letters)-limit+1:])
	}
	return dir
}

func (m model) homeView() string {
	// A terminal too narrow for the logo gets the name in plain letters.
	heading := []string{goodStyle.Bold(true).Render("taasant")}
	if m.width >= lipgloss.Width(logo[0]) {
		heading = nil
		for _, line := range logo {
			heading = append(heading, goodStyle.Render(line))
		}
	}
	about := goodStyle.Render("Your files, encrypted, in Telegram.")
	if m.width >= 70 {
		about += "  " + linkStyle.Render("github.com/vedantlavale/taasant")
	}
	status := dimStyle.Render(summary(m.entries) + " stored")
	if m.setup != nil {
		status = warnStyle.Render("Not set up yet: "+m.setup.Error()) + dimStyle.Render("  |  H shows what to set")
	}
	heading = append(heading, "", about, "", status, "")

	row := func(i int) string {
		text := fmt.Sprintf("%d. %-10s %s", i+1, menuItems[i].name, menuItems[i].about)
		if i == m.choice {
			return cursorStyle.Render("➤ " + text)
		}
		return "  " + text
	}
	return m.page(heading, m.choice, len(menuItems), row, dimStyle.Render("↑↓  |  Enter  |  H Help  |  Q Quit"))
}

func (m model) browseView() string {
	title := titleStyle.Render("Upload") + "  " + dimStyle.Render(short(m.dir, max(20, m.width-40)))
	enter := "Enter Open or upload"
	if len(m.marked) > 0 {
		var total int64
		for _, size := range m.marked {
			total += size
		}
		title += dimStyle.Render("  |  ") + goodStyle.Render(fmt.Sprintf("%d selected, %s", len(m.marked), formatSize(total)))
		enter = fmt.Sprintf("Enter Upload %d", len(m.marked))
	}

	width := 12
	for _, it := range m.items {
		width = max(width, lipgloss.Width(it.name))
	}
	width = min(width, 48, max(12, m.width-24))

	row := func(i int) string {
		it := m.items[i]
		pointer, mark, icon, look, size := "  ", dimStyle.Render("○ "), "📄", plain, formatSize(it.size)
		if _, marked := m.marked[filepath.Join(m.dir, it.name)]; marked {
			mark, look = goodStyle.Render("● "), goodStyle
		}
		if it.dir {
			mark, icon, look, size = "  ", "📁", folderStyle, ""
		}
		if i == m.cursor {
			pointer, look = cursorStyle.Render("➤ "), cursorStyle
		}
		return pointer + mark + icon + " " + look.Render(fit(it.name, width)) + dimStyle.Render(fmt.Sprintf("%12s", size))
	}
	keys := "↑↓←→  |  Space Select  |  " + enter + "  |  Esc Menu  |  Q Quit"
	return m.page([]string{title, badStyle.Render(m.notice)}, m.cursor, len(m.items), row, dimStyle.Render(keys))
}

func (m model) listView() string {
	title := menuItems[m.choice].name
	width := len("NAME")
	for _, e := range m.entries {
		width = max(width, lipgloss.Width(e.Name))
	}
	width = min(width, 48, max(12, m.width-32))
	columns := func(size, when string) string { return dimStyle.Render(fmt.Sprintf("  %10s   %s", size, when)) }

	heading := []string{
		titleStyle.Render(title) + "  " + dimStyle.Render(summary(m.entries)),
		badStyle.Render(m.notice),
		"  " + dimStyle.Render(fit("NAME", width)) + columns("SIZE", "UPLOADED"),
	}
	now := time.Now()
	row := func(i int) string {
		e := m.entries[i]
		pointer, look := "  ", plain
		if i == m.cursor {
			pointer, look = cursorStyle.Render("➤ "), cursorStyle
		}
		return pointer + look.Render(fit(e.Name, width)) + columns(formatSize(e.Size), formatWhen(e.Uploaded, now))
	}

	keys := "↑↓  |  Esc Menu  |  Q Quit"
	if m.action != "" {
		keys = "↑↓  |  Enter " + title + "  |  Esc Menu  |  Q Quit"
	}
	keys = dimStyle.Render(keys)
	if m.confirm {
		keys = badStyle.Render("Delete "+m.entries[m.cursor].Name+"?") + dimStyle.Render("  Y Yes  |  any other key No")
	}
	return m.page(heading, m.cursor, len(m.entries), row, keys)
}

func (m model) workView() string {
	spin := string([]rune(spinner)[m.frame%10])
	good, bad := 0, 0
	for _, j := range m.jobs {
		switch j.state {
		case succeeded:
			good++
		case failed:
			bad++
		}
	}

	status := fmt.Sprintf("%d of %d", m.current+1, len(m.jobs))
	switch {
	case m.quitting:
		status = spin + " stopping"
	case !m.busy:
		status = fmt.Sprintf("%d done", good)
		if bad > 0 {
			status += fmt.Sprintf(", %d failed", bad)
		}
	case m.jobs[m.current].state != running:
		status = spin + " backing up the list of files"
	}
	if m.action == "download" {
		status += "  |  into " + short(m.startDir, max(20, m.width-50))
	}

	width := 12
	for _, j := range m.jobs {
		width = max(width, lipgloss.Width(j.name))
	}
	width = min(width, 40, max(12, m.width-54))

	row := func(i int) string {
		j := m.jobs[i]
		name, size := fit(j.name, width), dimStyle.Render(fmt.Sprintf("  %10s", formatSize(j.size)))
		switch j.state {
		case running:
			line := "  " + cursorStyle.Render(spin) + " " + name
			if j.action == "delete" {
				return line
			}
			return line + "  " + bar(m.moved.Load(), j.size)
		case succeeded:
			return "  " + goodStyle.Render("✓") + " " + name + size
		case failed:
			reason := j.err.Error()
			if errors.Is(j.err, context.Canceled) {
				reason = "cancelled"
			}
			return "  " + badStyle.Render("✗ "+name+"  "+reason)
		}
		return "    " + dimStyle.Render(name) + size
	}

	keys := "Enter Menu  |  Q Quit"
	if m.busy {
		keys = "Esc Cancel  |  Q Quit"
	}
	heading := []string{
		titleStyle.Render(menuItems[m.choice].name) + "  " + dimStyle.Render(status),
		badStyle.Render(m.notice),
	}
	return m.page(heading, m.current, len(m.jobs), row, dimStyle.Render(keys))
}
