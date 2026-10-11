package portal

import (
	"strings"
	"testing"
)

func TestPageScrollsAndPortalStartsClosed(t *testing.T) {
	html := string(pageHTML)
	for _, want := range []string{
		"overflow-y: auto",
		"min-height: 100dvh",
		"minmax(0, 1fr)",
		"overflow-wrap: anywhere",
		`id="world" hidden`,
		`id="tray"`,
		`id="trail"`,
		"getUserMedia",
		"playsInline",
		"pagehide",
		"overTray",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %s", want)
		}
	}
	if strings.Contains(html, "height: 100vh") {
		t.Fatal("a locked 100vh height prevents the page from scrolling")
	}
	body := html[strings.Index(html, "html, body"):strings.Index(html, ".layout")]
	if strings.Contains(body, "overflow: hidden") {
		t.Fatal("page root hides overflow")
	}
}
