//go:build darwin

package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchAgentAutostartEnableDisable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := EnableAutostart(); err != nil {
		t.Fatal(err)
	}
	enabled, err := AutostartEnabled()
	if err != nil || !enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "so.beam.cli.plist")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plist := string(contents)
	if !strings.Contains(plist, "<key>RunAtLoad</key><true/>") {
		t.Fatal("LaunchAgent does not run at login")
	}
	if !strings.Contains(plist, "<string>--background</string>") {
		t.Fatal("LaunchAgent does not use background logging mode")
	}
	if strings.Contains(plist, "<key>KeepAlive</key>") {
		t.Fatal("LaunchAgent must not restart a crashed daemon")
	}

	if err := DisableAutostart(); err != nil {
		t.Fatal(err)
	}
	enabled, err = AutostartEnabled()
	if err != nil || enabled {
		t.Fatalf("enabled after disable=%v err=%v", enabled, err)
	}
}
