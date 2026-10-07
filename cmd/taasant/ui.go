package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Colours, symbols and the progress bar are only used when a person is
// looking at a terminal. Piped into a file or a script, output stays plain.
var (
	outTTY = isTerminal(os.Stdout)
	errTTY = isTerminal(os.Stderr)
)

const (
	dim   = "2"
	red   = "31"
	green = "32"
)

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func paint(on bool, code, text string) string {
	if !on || os.Getenv("NO_COLOR") != "" {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func printDone(message string) {
	if outTTY {
		message = paint(true, green, "✓") + " " + message
	}
	fmt.Println(message)
}

func printError(err error) {
	prefix := "error:"
	if errTTY {
		prefix = paint(true, red, "✗")
	}
	fmt.Fprintln(os.Stderr, prefix, err)
}

func printList(entries []entry, now time.Time) {
	width := len("NAME")
	for _, e := range entries {
		width = max(width, utf8.RuneCountInString(e.Name))
	}
	row := func(name, size, when string) string {
		padding := strings.Repeat(" ", width-utf8.RuneCountInString(name))
		return fmt.Sprintf("  %s%s  %10s   %s", name, padding, size, when)
	}

	fmt.Println(paint(outTTY, dim, row("NAME", "SIZE", "UPLOADED")))
	for _, e := range entries {
		fmt.Println(row(e.Name, formatSize(e.Size), formatWhen(e.Uploaded, now)))
	}
	fmt.Println()
	fmt.Println(paint(outTTY, dim, "  "+summary(entries)))
}

// summary counts the files and adds up their sizes: "3 files, 46.2 MiB".
func summary(entries []entry) string {
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	files := "files"
	if len(entries) == 1 {
		files = "file"
	}
	return fmt.Sprintf("%d %s, %s", len(entries), files, formatSize(total))
}

func formatWhen(t, now time.Time) string {
	t = t.In(now.Location())
	sameDay := func(a, b time.Time) bool {
		return a.Year() == b.Year() && a.YearDay() == b.YearDay()
	}
	switch {
	case sameDay(t, now):
		return "today " + t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "yesterday"
	case t.Year() == now.Year():
		return t.Format("2 Jan")
	default:
		return t.Format("2 Jan 2006")
	}
}

func formatDuration(d time.Duration) string {
	if d < 10*time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func shorten(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit-1]) + "…"
}

// bar draws how far done is towards total.
func bar(done, total int64) string {
	if total <= 0 {
		return formatSize(done)
	}
	const width = 20
	filled := int(min(done, total) * width / total)
	return paint(true, green, strings.Repeat("█", filled)) +
		paint(true, dim, strings.Repeat("░", width-filled)) +
		"  " + formatSize(done) + " / " + formatSize(total)
}

// showProgress returns a function that redraws one line of the terminal each
// time it is told how far a transfer is.
func showProgress(name string) func(done, total int64) {
	return func(done, total int64) {
		if errTTY {
			fmt.Fprint(os.Stderr, "\r\033[K  "+shorten(name, 30)+"  "+bar(done, total))
		}
	}
}

func clearProgress() {
	if errTTY {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}
