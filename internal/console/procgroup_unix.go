//go:build !windows

package console

import (
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func sendInterruptToProcessGroup(pgid int, pid int) error {
	if pid > 0 {
		_ = killProcessTreeBestEffort(pid, syscall.SIGINT)
	}
	if pgid > 0 {
		if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	return nil
}

func sendKillToProcessGroup(pgid int, pid int) error {
	if pid > 0 {
		_ = killProcessTreeBestEffort(pid, syscall.SIGKILL)
	}
	if pgid > 0 {
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	return nil
}

func processGroupExists(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	err := syscall.Kill(-pgid, 0)
	if err == nil {
		return true
	}
	// EPERM means it exists but we lack permission; treat as exists.
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

func killProcessTreeBestEffort(rootPID int, sig syscall.Signal) error {
	pids, err := listDescendantPIDsViaPS(rootPID)
	if err != nil {
		// Fall back to best-effort root kill.
		if err2 := syscall.Kill(rootPID, sig); err2 != nil && !errors.Is(err2, syscall.ESRCH) {
			return err2
		}
		return nil
	}

	// Kill children first.
	for i := len(pids) - 1; i >= 0; i-- {
		if err := syscall.Kill(pids[i], sig); err != nil && !errors.Is(err, syscall.ESRCH) {
			// Continue; best effort.
		}
	}
	if err := syscall.Kill(rootPID, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func listDescendantPIDsViaPS(rootPID int) ([]int, error) {
	cmd := exec.Command("ps", "-eo", "pid=,ppid=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	childrenByPPID := make(map[int][]int)
	lines := bytes.Split(out, []byte{'\n'})
	for _, line := range lines {
		fields := strings.Fields(string(line))
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		childrenByPPID[ppid] = append(childrenByPPID[ppid], pid)
	}

	var res []int
	seen := map[int]struct{}{rootPID: {}}
	queue := []int{rootPID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range childrenByPPID[cur] {
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			res = append(res, child)
			queue = append(queue, child)
		}
	}
	return res, nil
}
