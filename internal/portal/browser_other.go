//go:build !darwin && !windows

package portal

import "os/exec"

func OpenBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}
