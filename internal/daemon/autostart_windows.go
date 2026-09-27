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
	command := fmt.Sprintf(`"%s" daemon --background`, exe)
	return hiddenCommand(
		"schtasks", "/Create", "/SC", "ONLOGON", "/TN", "BEAM",
		"/TR", command, "/RL", "LIMITED", "/F",
	).Run()
}

func DisableAutostart() error {
	err := hiddenCommand("schtasks", "/Delete", "/TN", "BEAM", "/F").Run()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return nil
	}
	return err
}

func AutostartEnabled() (bool, error) {
	err := hiddenCommand("schtasks", "/Query", "/TN", "BEAM").Run()
	if err == nil {
		return true, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
