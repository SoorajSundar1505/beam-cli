package cli

import (
	"bytes"
	"strings"
	"testing"

	"beam/internal/device"
)

func TestDeviceColumnsUseTheSameLayout(t *testing.T) {
	items := []device.Listed{
		{Name: "MacBook", Status: "this device", Self: true},
		{Name: "Windows-PC", Status: "online"},
		{Name: "Work-Mac", Status: "offline"},
	}
	var out bytes.Buffer
	renderDeviceList(&out, true, "MacBook", items)
	text := out.String()
	want := "" +
		"BEAM [ONLINE]  MacBook\n\n" +
		"Devices\n\n" +
		"1. MacBook     [THIS DEVICE]\n" +
		"2. Windows-PC  [ONLINE]\n" +
		"3. Work-Mac    [OFFLINE]\n"
	if text != want {
		t.Fatalf("device layout:\n%q", text)
	}
	if strings.Count(text, "MacBook") != 2 || strings.Count(text, "[THIS DEVICE]") != 1 {
		t.Fatalf("current device was duplicated or omitted:\n%s", text)
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var mac, windows, work string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "1. MacBook"):
			mac = line
		case strings.Contains(line, "Windows-PC"):
			windows = line
		case strings.Contains(line, "Work-Mac"):
			work = line
		}
	}
	if strings.Index(mac, "[") != strings.Index(windows, "[") || strings.Index(mac, "[") != strings.Index(work, "[") {
		t.Fatalf("status columns are not aligned:\n%s\n%s\n%s", mac, windows, work)
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
	want := "" +
		"BEAM [ONLINE]  MacBook\n" +
		"daemon: running | autostart: on\n\n" +
		"Devices\n\n" +
		"1. MacBook     [THIS DEVICE]\n" +
		"2. Windows-PC  [ONLINE]\n"
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
