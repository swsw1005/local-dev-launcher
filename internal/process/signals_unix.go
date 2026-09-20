//go:build darwin || linux

package process

import (
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
		return syscall.Kill(-pgid, signal)
	}
	return syscall.Kill(pid, signal)
}

func processAlive(pid, pgid int) bool {
	if pgid > 0 {
		return syscall.Kill(-pgid, 0) == nil
	}
	return syscall.Kill(pid, 0) == nil
}
