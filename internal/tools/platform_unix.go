//go:build !windows

package tools

import (
	"os"
	"os/exec"
	"syscall"
)

func executable(name string, args ...string) (*exec.Cmd, error) {
	path, err := LookPath(name)
	if err != nil {
		return nil, err
	}
	return exec.Command(path, args...), nil
}

func Shell(path, command string) *exec.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	var cmd *exec.Cmd
	if command == "" {
		cmd = exec.Command(shell)
	} else {
		cmd = exec.Command(shell, "-c", command)
	}
	cmd.Dir = path
	return cmd
}

func background(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
