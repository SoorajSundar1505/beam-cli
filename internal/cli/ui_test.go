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
	renderDevices(&out, true, "Windows-PC", nil, items)
	text := out.String()
	for _, want := range []string{
		"BEAM [ONLINE]",
		"Device: Windows-PC",
		"Nearby devices",
		"1. MacBook   [OFFLINE]",
		"2. Work-Mac  [ONLINE]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
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

func TestStatusAndDevicesShareLabels(t *testing.T) {
	items := []device.Listed{
		{Name: "Windows-PC", Status: "this device", Self: true},
		{Name: "MacBook", Status: "offline"},
	}
	var devices, status bytes.Buffer
	renderDevices(&devices, true, "Windows-PC", nil, items)
	renderDevices(&status, true, "Windows-PC", []string{
		"Daemon: running (PID 19096)",
		"Autostart: enabled",
		"Queued transfers: 0",
	}, items)
	for _, text := range []string{devices.String(), status.String()} {
		if !strings.Contains(text, "BEAM [ONLINE]") || !strings.Contains(text, "1. MacBook  [OFFLINE]") {
			t.Fatalf("layout drifted:\n%s", text)
		}
	}
	if !strings.Contains(status.String(), "Daemon: running (PID 19096)") {
		t.Fatalf("status details missing:\n%s", status.String())
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
