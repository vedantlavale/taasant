package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type entry struct {
	Name     string    `json:"name"`
	ID       string    `json:"id"`
	Size     int64     `json:"size"`
	Uploaded time.Time `json:"uploaded"`
}

func (e entry) String() string {
	details := formatSize(e.Size) + "  " + formatWhen(e.Uploaded, time.Now())
	return fmt.Sprintf("  %s  %s\n    %s", e.Name, paint(errTTY, dim, details), paint(errTTY, dim, e.ID))
}

func formatSize(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	size := float64(n)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}

func indexPath() (string, error) {
	if path := os.Getenv("TAAS_INDEX"); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "taas", "index.json"), nil
}

func loadIndex(path string) ([]entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []entry
	err = json.Unmarshal(data, &entries)
	return entries, err
}

func saveIndex(path string, entries []entry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

// fuzzyMatch reports whether the letters of query appear in name in the same order.
func fuzzyMatch(query, name string) bool {
	name = strings.ToLower(name)
	for _, letter := range strings.ToLower(query) {
		i := strings.IndexRune(name, letter)
		if i < 0 {
			return false
		}
		name = name[i+len(string(letter)):]
	}
	return true
}

// search returns the best kind of match that finds anything:
// the exact name, then names containing query, then fuzzy matches.
func search(entries []entry, query string) []entry {
	rules := []func(name string) bool{
		func(name string) bool { return name == query },
		func(name string) bool { return strings.Contains(strings.ToLower(name), strings.ToLower(query)) },
		func(name string) bool { return fuzzyMatch(query, name) },
	}
	for _, matches := range rules {
		var found []entry
		for _, e := range entries {
			if matches(e.Name) {
				found = append(found, e)
			}
		}
		if len(found) > 0 {
			slices.SortFunc(found, func(a, b entry) int {
				if a.Uploaded.After(b.Uploaded) {
					return -1
				}
				if a.Uploaded.Before(b.Uploaded) {
					return 1
				}
				return 0
			})
			return found
		}
	}
	return nil
}

// find resolves what the user typed, a name, part of a name or an ID, to one entry.
func find(entries []entry, query string) (entry, error) {
	for _, e := range entries {
		if e.ID == query {
			return e, nil
		}
	}

	found := search(entries, query)
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) == 0 {
		if strings.Contains(query, ":") {
			return entry{ID: query}, nil
		}
		return entry{}, fmt.Errorf("nothing in the index matches %q", query)
	}

	lines := []string{fmt.Sprintf("%d files match %q, type more of the name or use the ID:", len(found), query)}
	for _, e := range found {
		lines = append(lines, e.String())
	}
	return entry{}, errors.New(strings.Join(lines, "\n"))
}
