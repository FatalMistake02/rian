package tools

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func executable(name string, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(path), ".cmd") || strings.HasSuffix(strings.ToLower(path), ".bat") {
		// CmdLine avoids Go's CRT argument escaping, which cmd.exe does not use.
		// Reject expansion characters in generated arguments to keep paths literal.
		all := append([]string{path}, args...)
		for i, arg := range all {
			if strings.ContainsAny(arg, "\"%\r\n") {
				return nil, fmt.Errorf("batch launcher cannot safely pass this path: %s", arg)
			}
			all[i] = "\"" + arg + "\""
		}
		cmd := exec.Command("cmd.exe", "/d", "/v:off", "/s", "/c", strings.Join(all, " "))
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /v:off /s /c \"" + strings.Join(all, " ") + "\""}
		return cmd, nil
	}
	return exec.Command(path, args...), nil
}

func Shell(path, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if command != "" {
		cmd = exec.Command("cmd.exe", "/d", "/s", "/c", command)
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d /s /c \"" + command + "\""}
	} else if pwsh, err := exec.LookPath("pwsh"); err == nil {
		cmd = exec.Command(pwsh, "-NoLogo")
	} else {
		cmd = exec.Command("powershell.exe", "-NoLogo")
	}
	cmd.Dir = path
	return cmd
}

func background(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x00000200 // CREATE_NEW_PROCESS_GROUP
}
