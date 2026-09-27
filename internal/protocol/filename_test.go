package protocol

import "testing"

func TestSafeFileName(t *testing.T) {
	ok, err := SafeFileName("photo.jpg")
	if err != nil || ok != "photo.jpg" {
		t.Fatalf("got %q %v", ok, err)
	}
	ok, err = SafeFileName("/tmp/evil.txt")
	if err != nil || ok != "evil.txt" {
		t.Fatalf("base should strip dirs: %q %v", ok, err)
	}
	for _, name := range []string{"", ".", "..", "foo/../bar", "a\x00b"} {
		if _, err := SafeFileName(name); err == nil && name != "foo/../bar" {
			t.Fatalf("expected error for %q", name)
		}
	}
	// filepath.Base("foo/../bar") == "bar"
	got, err := SafeFileName("foo/../bar")
	if err != nil || got != "bar" {
		t.Fatalf("expected bar, got %q %v", got, err)
	}
}
