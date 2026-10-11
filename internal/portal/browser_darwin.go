//go:build darwin

package portal

import "os/exec"

func OpenBrowser(url string) error {
	return exec.Command("open", url).Start()
}
