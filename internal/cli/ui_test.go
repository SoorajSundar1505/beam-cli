package cli

import (
	"bytes"
	"strings"
	"testing"

	"beam/internal/device"
)

func TestDeviceColumnsUseTheSameLayout(t *testing.T) {
	items := []device.Listed{
		{Name: "Windows-PC", Status: "this device", Self: true},
		{Name: "MacBook", Status: "offline"},
		{Name: "Work-Mac", Status: "online"},
	}
	var out bytes.Buffer
	renderNearby(&out, items)
	text := out.String()
	for _, want := range []string{
		"Nearby\n",
		"1. MacBook   [OFFLINE]",
		"2. Work-Mac  [ONLINE]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "BEAM") || strings.Contains(text, "daemon:") {
		t.Fatalf("device list includes status chrome:\n%s", text)
	}
	if strings.Contains(text, "this device") || strings.Contains(text, "*") || strings.Contains(text, " o ") {
		t.Fatalf("nearby list uses a status marker outside the shared labels:\n%s", text)
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var mac, work string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "MacBook"):
			mac = line
		case strings.Contains(line, "Work-Mac"):
			work = line
		}
	}
	if mac == "" || work == "" {
		t.Fatalf("device rows missing:\n%s", text)
	}
	if strings.Index(mac, "[") != strings.Index(work, "[") {
		t.Fatalf("status columns are not aligned:\n%s\n%s", mac, work)
	}
	if strings.ContainsAny(text, "●○✓✗█░→*") {
		t.Fatalf("output contains unreliable symbols:\n%s", text)
	}
}

func TestStatusIsCompact(t *testing.T) {
	items := []device.Listed{
		{Name: "MacBook", Status: "this device", Self: true},
		{Name: "Windows-PC", Status: "online"},
	}
	var out bytes.Buffer
	renderStatus(&out, true, "MacBook", true, items, nil)
	text := out.String()
	want := "BEAM [ONLINE]  MacBook\ndaemon: running | autostart: on\n\nNearby\n1. Windows-PC  [ONLINE]\n"
	if text != want {
		t.Fatalf("status layout:\n%q", text)
	}
	if strings.Contains(text, "PID") || strings.Contains(text, "Queued") || strings.Contains(text, "pid:") {
		t.Fatalf("normal status includes diagnostics:\n%s", text)
	}
}

func TestStatusVerboseKeepsDiagnostics(t *testing.T) {
	items := []device.Listed{{Name: "MacBook", Status: "this device", Self: true}}
	var out bytes.Buffer
	renderStatus(&out, true, "MacBook", true, items, []string{"pid: 17910", "queued: 0", "log: /tmp/beam.log"})
	text := out.String()
	for _, want := range []string{"BEAM [ONLINE]  MacBook", "daemon: running | autostart: on", "pid: 17910", "queued: 0", "log: /tmp/beam.log"} {
		if !strings.Contains(text, want) {
			t.Fatalf("verbose status missing %q\n%s", want, text)
		}
	}
}

func TestClipboardColumnsAlign(t *testing.T) {
	text := formatClipboard([]row{
		{number: 1, name: "permissions", state: "2 min ago"},
		{number: 2, name: "actions", state: "12 min ago"},
	})
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) != 2 || strings.Index(lines[0], "2 min ago") != strings.Index(lines[1], "12 min ago") {
		t.Fatalf("clipboard times are not aligned:\n%s", text)
	}
}
