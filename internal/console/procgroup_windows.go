//go:build windows

package console

import (
	"os"
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Unix-style process groups via SysProcAttr.Setpgid.
}

func sendInterruptToProcessGroup(pgid int, pid int) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = p.Signal(os.Interrupt)
	return nil
}

func sendKillToProcessGroup(pgid int, pid int) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = p.Kill()
	return nil
}

func processGroupExists(pgid int) bool {
	return false
}
