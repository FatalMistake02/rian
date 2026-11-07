package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x00000200
}

func stopTree(cmd *exec.Cmd) error {
	kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := kill.CombinedOutput(); err != nil {
		return fmt.Errorf("could not stop process tree: %s (%w)", output, err)
	}
	return nil
}
