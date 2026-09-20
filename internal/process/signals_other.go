//go:build !darwin && !linux

package process

import (
	"os"
	"os/exec"
)

func configureProcessGroup(command *exec.Cmd) {}

func processGroupID(pid int) int { return 0 }

func terminateProcess(pid, pgid int, force bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func processAlive(pid, pgid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process != nil
}
