package main

// shouldDetachConsole reports whether this process owns a console that
// exists only for a background daemon, such as the login autostart. A
// daemon started from CMD or PowerShell shares that shell's console and
// must leave it alone. A daemon started with CREATE_NO_WINDOW has no
// console (count 0) and must not attach to the caller's console either.
func shouldDetachConsole(args []string, consoleProcesses int) bool {
	return consoleProcesses == 1 && commandIsDaemon(args)
}

func commandIsDaemon(args []string) bool {
	for _, arg := range args[1:] {
		if arg == "" || arg[0] == '-' {
			continue
		}
		return arg == "daemon"
	}
	return false
}
