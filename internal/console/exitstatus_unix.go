//go:build !windows

package console

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
)

func parseUnixExitStatus(err error) (exitCode int, signal string, ok bool) {
	if err == nil {
		return 0, "", true
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return 0, "", false
	}
	ws, okWait := ee.Sys().(syscall.WaitStatus)
	if !okWait {
		return 0, "", false
	}
	if ws.Signaled() {
		return 0, normalizeSignalName(ws.Signal()), true
	}
	if ws.Exited() {
		return ws.ExitStatus(), "", true
	}
	return 0, "", false
}

func normalizeSignalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGKILL:
		return "SIGKILL"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		// Best effort; may be "SIGABRT" etc on some platforms.
		return "SIG" + strings.ToUpper(sig.String())
	}
}
