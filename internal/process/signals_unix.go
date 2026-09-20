//go:build darwin || linux

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func processGroupID(pid int) int { return pid }

func terminateProcess(pid, pgid int, force bool) error {
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	if pgid > 0 {
		err := syscall.Kill(-pgid, signal)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	err := syscall.Kill(pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

func processAlive(pid, pgid int) bool {
	if pgid > 0 {
		return syscall.Kill(-pgid, 0) == nil
	}
	return syscall.Kill(pid, 0) == nil
}
