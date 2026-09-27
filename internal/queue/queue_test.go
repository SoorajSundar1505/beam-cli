package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnqueueCopiesAndRemovesFile(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	src := filepath.Join(t.TempDir(), "report.pdf")
	want := []byte("stable queued content")
	if err := os.WriteFile(src, want, 0o600); err != nil {
		t.Fatal(err)
	}
	entry, err := EnqueueFile(src, "peer-id", "Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(entry.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("queue did not preserve content: %q", got)
	}
	entries, err := List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("list: %d, %v", len(entries), err)
	}
	if entries[0].Name != "report.pdf" || entries[0].TargetName != "Windows-PC" {
		t.Fatalf("bad entry: %+v", entries[0])
	}
	if err := Remove(entries[0]); err != nil {
		t.Fatal(err)
	}
	entries, err = List()
	if err != nil || len(entries) != 0 {
		t.Fatalf("remove: %d, %v", len(entries), err)
	}
}
