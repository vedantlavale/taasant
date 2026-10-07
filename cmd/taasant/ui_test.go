package main

import (
	"testing"
	"time"
)

func TestFormatWhen(t *testing.T) {
	now := time.Date(2026, time.October, 6, 15, 30, 0, 0, time.UTC)
	tests := []struct {
		when time.Time
		want string
	}{
		{time.Date(2026, time.October, 6, 9, 5, 0, 0, time.UTC), "today 09:05"},
		{time.Date(2026, time.October, 5, 23, 59, 0, 0, time.UTC), "yesterday"},
		{time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC), "3 Oct"},
		{time.Date(2025, time.December, 31, 12, 0, 0, 0, time.UTC), "31 Dec 2025"},
	}
	for _, test := range tests {
		if got := formatWhen(test.when, now); got != test.want {
			t.Errorf("formatWhen(%v) = %q, want %q", test.when, got, test.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		400 * time.Millisecond:   "0.4s",
		9500 * time.Millisecond:  "9.5s",
		12400 * time.Millisecond: "12s",
		72 * time.Second:         "1m12s",
	}
	for d, want := range tests {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestBar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		done, total int64
		want        string
	}{
		{0, 10, "░░░░░░░░░░░░░░░░░░░░  0 B / 10 B"},
		{5, 10, "██████████░░░░░░░░░░  5 B / 10 B"},
		{30, 10, "████████████████████  30 B / 10 B"},
		{7, 0, "7 B"},
	}
	for _, test := range tests {
		if got := bar(test.done, test.total); got != test.want {
			t.Errorf("bar(%d, %d) = %q, want %q", test.done, test.total, got, test.want)
		}
	}
}

func TestShorten(t *testing.T) {
	tests := []struct {
		text  string
		limit int
		want  string
	}{
		{"short.txt", 30, "short.txt"},
		{"abcdefghij", 10, "abcdefghij"},
		{"abcdefghijk", 10, "abcdefghi…"},
		{"世界世界世界", 4, "世界世…"},
	}
	for _, test := range tests {
		if got := shorten(test.text, test.limit); got != test.want {
			t.Errorf("shorten(%q, %d) = %q, want %q", test.text, test.limit, got, test.want)
		}
	}
}
