package main

import (
	"path/filepath"
	"testing"
	"time"
)

var sample = []entry{
	{Name: "holiday.jpg", ID: "1:a"},
	{Name: "holiday-2.jpg", ID: "2:b"},
	{Name: "report.pdf", ID: "3:c"},
	{Name: "Notes.txt", ID: "4:d"},
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		query, name string
		want        bool
	}{
		{"rpt", "report.pdf", true},
		{"RPT", "report.pdf", true},
		{"nts", "Notes.txt", true},
		{"", "anything", true},
		{"tpr", "report.pdf", false},
		{"reportt", "report.pdf", false},
	}
	for _, test := range tests {
		if got := fuzzyMatch(test.query, test.name); got != test.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", test.query, test.name, got, test.want)
		}
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"holiday.jpg", []string{"holiday.jpg"}},
		{"holiday", []string{"holiday.jpg", "holiday-2.jpg"}},
		{"notes", []string{"Notes.txt"}},
		{"rpt", []string{"report.pdf"}},
		{"", []string{"holiday.jpg", "holiday-2.jpg", "report.pdf", "Notes.txt"}},
		{"zzz", nil},
	}
	for _, test := range tests {
		found := search(sample, test.query)
		if len(found) != len(test.want) {
			t.Errorf("search(%q) found %d entries, want %d", test.query, len(found), len(test.want))
			continue
		}
		for i, e := range found {
			if e.Name != test.want[i] {
				t.Errorf("search(%q)[%d] = %q, want %q", test.query, i, e.Name, test.want[i])
			}
		}
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		query  string
		wantID string
	}{
		{"rpt", "3:c"},
		{"holiday.jpg", "1:a"},
		{"2:b", "2:b"},
		{"99:unknown", "99:unknown"},
		{"holiday", ""},
		{"zzz", ""},
	}
	for _, test := range tests {
		e, err := find(sample, test.query)
		if test.wantID == "" {
			if err == nil {
				t.Errorf("find(%q) succeeded with %q, want an error", test.query, e.ID)
			}
			continue
		}
		if err != nil || e.ID != test.wantID {
			t.Errorf("find(%q) = %q, %v, want %q", test.query, e.ID, err, test.wantID)
		}
	}
}

func TestSaveAndLoadIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "index.json")

	entries, err := loadIndex(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("loading a missing index gave %v, %v", entries, err)
	}

	saved := []entry{{Name: "a.txt", ID: "1:a", Size: 42, Uploaded: time.Unix(1700000000, 0)}}
	if err := saveIndex(path, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Name != "a.txt" || loaded[0].ID != "1:a" || loaded[0].Size != 42 || !loaded[0].Uploaded.Equal(saved[0].Uploaded) {
		t.Fatalf("loaded %+v, want %+v", loaded, saved)
	}
}

func TestFormatSize(t *testing.T) {
	tests := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 20 << 20: "20.0 MiB"}
	for n, want := range tests {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}
