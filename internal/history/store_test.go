package history

import (
	"testing"
	"time"
)

func TestHistorySearch(t *testing.T) {
	s, err := OpenPath(t.TempDir() + "/clip.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	entries := []Item{
		{Kind: KindText, Text: "https://beam.so", CreatedAt: now.Add(-2 * time.Minute)},
		{Kind: KindText, Text: "Meeting at 5pm", CreatedAt: now.Add(-12 * time.Minute)},
		{Kind: KindText, Text: "API documentation", CreatedAt: now.Add(-time.Hour)},
		{Kind: KindFile, Filename: "image.png", CreatedAt: now.Add(-3 * time.Hour)},
		{Kind: KindText, Text: "Meeting notes - Q3", CreatedAt: now.Add(-48 * time.Hour)},
		{Kind: KindText, Text: "Team meeting link", CreatedAt: now.Add(-72 * time.Hour)},
	}
	for _, e := range entries {
		if _, err := s.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(10)
	if err != nil || len(list) != 6 {
		t.Fatalf("list %d %v", len(list), err)
	}
	if list[0].Text != "https://beam.so" {
		t.Fatalf("order %q", list[0].Text)
	}
	found, err := s.Search("meeting", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 3 {
		t.Fatalf("expected meeting matches, got %d %#v", len(found), found)
	}
	got, err := s.Get(list[1].ID)
	if err != nil || got.Text != "Meeting at 5pm" {
		t.Fatalf("get %+v %v", got, err)
	}
}

func TestSearchMatchesStoredClipboardText(t *testing.T) {
	s, err := OpenPath(t.TempDir() + "/search.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now()
	entries := []Item{
		{Kind: KindText, Text: "permissions", CreatedAt: now.Add(-5 * time.Minute)},
		{Kind: KindText, Text: "actions", CreatedAt: now.Add(-4 * time.Minute)},
		{Kind: KindText, Text: "GitHub Actions workflow", CreatedAt: now.Add(-3 * time.Minute)},
		{Kind: KindText, Text: "Meeting notes", CreatedAt: now.Add(-2 * time.Minute)},
		{Kind: KindText, Text: "team meeting link", CreatedAt: now.Add(-time.Minute)},
	}
	for _, entry := range entries {
		if _, err := s.Add(entry); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "exact match", query: "permissions", want: []string{"permissions"}},
		{name: "partial match", query: "work", want: []string{"GitHub Actions workflow"}},
		{name: "case insensitive match", query: "ACTIONS", want: []string{"GitHub Actions workflow", "actions"}},
		{name: "no results", query: "does-not-exist", want: nil},
		{name: "multiple matching items", query: "meeting", want: []string{"team meeting link", "Meeting notes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Search(tt.query, 50)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Search(%q) returned %d items, want %d: %#v", tt.query, len(got), len(tt.want), got)
			}
			for i, want := range tt.want {
				if got[i].Text != want {
					t.Errorf("result %d = %q, want %q", i, got[i].Text, want)
				}
			}
		})
	}
}
