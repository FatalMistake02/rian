package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// LookPath also finds VS Code's bundled macOS launcher. Installing its shell
// command manually is optional when the app is in an Applications directory.
func LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	if runtime.GOOS == "darwin" && name == "code" {
		if app := applicationPath("Visual Studio Code", applicationDirectories()); app != "" {
			return exec.LookPath(filepath.Join(app, "Contents", "Resources", "app", "bin", "code"))
		}
	}
	return "", err
}

func applicationDirectories() []string {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

func applicationPath(name string, dirs []string) string {
	for _, dir := range dirs {
		path := filepath.Join(dir, name+".app")
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}
