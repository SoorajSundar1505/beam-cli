//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
