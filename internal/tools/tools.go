// Package tools isolates launchers from the interface. Interactive tools return
// an exec.Cmd that Bubble Tea can run while suspending its terminal renderer.
package tools

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"rian/internal/git"
	"rian/internal/project"
)

type Launcher interface {
	OpenURL(string) error
	OpenEditor(project.Project) error
	Interactive(string, project.Project) (*exec.Cmd, error)
}

type System struct{}

func checkDirectory(p project.Project) error {
	info, err := os.Stat(p.Path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("this directory is no longer available: %s", p.Path)
	}
	return nil
}

func (System) OpenEditor(p project.Project) error {
	if err := checkDirectory(p); err != nil {
		return err
	}
	cmd, err := executable("code", p.Path)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("VS Code was not found in PATH; install its 'code' launcher")
		}
		return err
	}
	cmd.Dir = p.Path
	return detach(cmd)
}

func (System) Interactive(name string, p project.Project) (*exec.Cmd, error) {
	if err := checkDirectory(p); err != nil {
		return nil, err
	}
	if name == "terminal" {
		return Shell(p.Path, ""), nil
	}
	if name == "kilocode" {
		if _, err := exec.LookPath("kilo"); err == nil {
			name = "kilo"
		}
	}
	cmd, err := executable(name)
	if err != nil {
		return nil, fmt.Errorf("%s was not found in PATH", name)
	}
	cmd.Dir = p.Path
	return cmd, nil
}

func GitHubURL(p project.Project, status git.Status) (string, error) {
	repo := p.GitHub
	if repo == "" {
		repo = status.GitHub
	}
	if repo == "" {
		return "", fmt.Errorf("this project has no GitHub remote; add an override in Edit Project")
	}
	repo, err := git.NormalizeRepo(repo)
	if err != nil {
		return "", err
	}
	return "https://github.com/" + repo, nil
}

func ValidateURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("use a complete http:// or https:// URL")
	}
	return nil
}

func (System) OpenURL(value string) error {
	if err := ValidateURL(value); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", value)
	case "darwin":
		cmd = exec.Command("open", value)
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", value)
		} else {
			cmd = exec.Command("gio", "open", value)
		}
	}
	return detach(cmd)
}

func detach(cmd *exec.Cmd) error {
	background(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the child without tying a graphical application's lifetime to Rian.
	go func() { _ = cmd.Wait() }()
	return nil
}

func GitCommand(p project.Project, args ...string) (*exec.Cmd, error) {
	if err := checkDirectory(p); err != nil {
		return nil, err
	}
	cmd, err := executable("git", args...)
	if err != nil {
		return nil, fmt.Errorf("Git was not found in PATH")
	}
	cmd.Dir = p.Path
	return cmd, nil
}

func CommandPreview(command string) string { return strings.TrimSpace(command) }

// ExternalCommand uses the platform's launcher rules, including Windows batch
// launchers, and passes all supplied arguments literally.
func ExternalCommand(p project.Project, name string, args ...string) (*exec.Cmd, error) {
	if err := checkDirectory(p); err != nil {
		return nil, err
	}
	cmd, err := executable(name, args...)
	if err != nil {
		return nil, err
	}
	cmd.Dir = p.Path
	return cmd, nil
}

func OpenApplication(cmd *exec.Cmd) error { return detach(cmd) }

func BrowserAvailable() bool {
	names := []string{"xdg-open", "gio"}
	if runtime.GOOS == "darwin" {
		names = []string{"open"}
	}
	if runtime.GOOS == "windows" {
		names = []string{"rundll32.exe"}
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}
