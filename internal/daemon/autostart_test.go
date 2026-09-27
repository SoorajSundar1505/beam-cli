package daemon

import (
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
