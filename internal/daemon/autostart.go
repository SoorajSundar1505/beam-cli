package daemon

import "strings"

// LoginCommand is the exact per-user login command. It starts one daemon
// process and does not invoke the short-lived CLI launcher.
func LoginCommand(executable string) string {
	return `"` + strings.ReplaceAll(executable, `"`, "") + `" daemon`
}
