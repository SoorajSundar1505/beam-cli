package daemon

import (
	"os"
	"strings"
	"testing"
)

func TestLoginCommandStartsOneDaemon(t *testing.T) {
	got := LoginCommand(`C:\Users\Ada\beam.exe`)
	if got != `"C:\Users\Ada\beam.exe" daemon` {
		t.Fatalf("command = %s", got)
	}
	lower := strings.ToLower(got)
	for _, forbidden := range []string{"schtasks", "powershell", "cmd.exe", "wscript", "--background"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("login command contains %s: %s", forbidden, got)
		}
	}
}

func TestWindowsAutostartIsPerUserRunKey(t *testing.T) {
	body, err := os.ReadFile("autostart_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"CURRENT_USER", `CurrentVersion\Run`, "LoginCommand"} {
		if !strings.Contains(text, want) {
			t.Fatalf("windows autostart missing %q", want)
		}
	}
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"schtasks", "powershell", "wscript", "detached_process"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("windows autostart contains %s", forbidden)
		}
	}
}
