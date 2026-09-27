//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func detach(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW gives this one daemon process its own non-visible
	// console lifetime. It is not used to launch any helper executable.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}

func terminate(p *os.Process) error {
	return p.Kill()
}
