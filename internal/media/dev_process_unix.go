//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package media

import (
	"os/exec"
	"syscall"
)

func configureDevChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptDevChild(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}

func killDevChild(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
