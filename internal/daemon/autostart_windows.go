//go:build windows

package daemon

import (
	"fmt"
	"os/exec"
)

func EnableAutostart() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	command := fmt.Sprintf(`"%s" daemon`, exe)
	return exec.Command("schtasks", "/Create", "/SC", "ONLOGON", "/TN", "BEAM", "/TR", command, "/F").Run()
}

func DisableAutostart() error {
	err := exec.Command("schtasks", "/Delete", "/TN", "BEAM", "/F").Run()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return nil
	}
	return err
}
