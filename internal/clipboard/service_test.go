package clipboard

import (
	"testing"

	"beam/internal/history"
)

type memPlat struct {
	cur *Item
}

func (m *memPlat) Read() (*Item, error) { return m.cur, nil }
func (m *memPlat) Write(it *Item) error { m.cur = it; return nil }

func TestClipboardHistoryAndSearch(t *testing.T) {
	st, err := history.OpenPath(t.TempDir() + "/c.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	p := &memPlat{cur: TextItem("hello world")}
	svc := New(p, st)
	if _, err := svc.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Snapshot(); err != nil {
		t.Fatal(err)
	}
	list, _ := st.List(10)
	if len(list) != 1 {
		t.Fatalf("dedup failed: %d", len(list))
	}
	p.cur = TextItem("Meeting at 5pm")
	if _, err := svc.Snapshot(); err != nil {
		t.Fatal(err)
	}
	found, err := st.Search("meet", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("search %d %v", len(found), err)
	}
}
