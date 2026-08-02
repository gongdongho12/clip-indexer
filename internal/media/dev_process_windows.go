//go:build windows

package media

import "os/exec"

func configureDevChild(_ *exec.Cmd) {}

func interruptDevChild(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func killDevChild(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
