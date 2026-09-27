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
