package tools

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func terminalBackend() string {
	names := []string{"konsole", "gnome-terminal", "xterm"}
	if runtime.GOOS == "windows" {
		names = []string{"wt.exe"}
	}
	if runtime.GOOS == "darwin" {
		names = []string{"osascript"}
	}
	for _, name := range names {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

func TerminalModes() []string {
	modes := []string{"same"}
	backend := terminalBackend()
	if backend != "" {
		modes = append(modes, "window")
	}

	return modes
}

// TerminalCommand wraps argv directly, preserving paths and arguments literally.
func TerminalCommand(child *exec.Cmd, mode string) (*exec.Cmd, error) {
	return terminalCommand(terminalBackend(), child, mode)
}
func terminalCommand(backend string, child *exec.Cmd, mode string) (*exec.Cmd, error) {
	if mode != "window" {
		return nil, fmt.Errorf("invalid terminal mode %q", mode)
	}
	args := []string{}
	switch backend {
	case "konsole":
		args = append(args, "--separate")
		args = append(args, "--hold", "--workdir", child.Dir, "-e")
	case "gnome-terminal":
		args = append(args, "--window", "--working-directory="+child.Dir, "--wait", "--")
	case "xterm":
		args = append(args, "-hold", "-e")
	case "wt.exe":
		args = append(args, "-w", "new", "new-tab", "-d", child.Dir)
	case "osascript":
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
		words := []string{}
		for _, arg := range child.Args {
			words = append(words, quote(arg))
		}
		script := "cd " + quote(child.Dir) + " && " + strings.Join(words, " ")
		cmd := exec.Command(backend, "-e", "tell application \"Terminal\"", "-e", "activate", "-e", "do script "+strconv.Quote(script), "-e", "end tell")
		return cmd, nil
	default:
		return nil, fmt.Errorf("no supported terminal found; choose Same terminal in Settings")
	}
	args = append(args, child.Args...)
	cmd := exec.Command(backend, args...)
	cmd.Dir, cmd.Env = child.Dir, child.Env
	return cmd, nil
}

func OpenTerminal(child *exec.Cmd, mode string) error {
	cmd, err := TerminalCommand(child, mode)
	if err != nil {
		return err
	}
	return detach(cmd)
}
