package tools

import (
	"fmt"
	"os/exec"
	"runtime"

	"rian/internal/project"
)

func filesCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path), nil
	case "darwin":
		return exec.Command("open", path), nil
	default:
		if command, err := exec.LookPath("xdg-open"); err == nil {
			return exec.Command(command, path), nil
		}
		if command, err := exec.LookPath("gio"); err == nil {
			return exec.Command(command, "open", path), nil
		}
		return nil, fmt.Errorf("no default file manager launcher found")
	}
}
func FilesAvailable() bool { _, err := filesCommand("."); return err == nil }
func (System) OpenFiles(p project.Project) error {
	if err := checkDirectory(p); err != nil {
		return err
	}
	cmd, err := filesCommand(p.Path)
	if err != nil {
		return err
	}
	return detach(cmd)
}
