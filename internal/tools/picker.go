package tools

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// FolderPicker returns a graphical directory chooser, when one is available.
func FolderPicker() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "Select a project folder")`), nil
	case "windows":
		return exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $picker = New-Object System.Windows.Forms.FolderBrowserDialog; $picker.Description = 'Select a project folder'; $picker.ShowNewFolderButton = $false; if ($picker.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; [Console]::Write($picker.SelectedPath) }; $picker.Dispose()`), nil
	default:
		if path, err := exec.LookPath("zenity"); err == nil {
			return exec.Command(path, "--file-selection", "--directory", "--title=Select a project folder"), nil
		}
		if path, err := exec.LookPath("kdialog"); err == nil {
			return exec.Command(path, "--getexistingdirectory", ".", "--title", "Select a project folder"), nil
		}
		return nil, fmt.Errorf("no folder picker available; type a directory path")
	}
}

// PickFolder returns an empty path when the user cancels the chooser.
func PickFolder() (string, error) {
	cmd, err := FolderPicker()
	if err != nil {
		return "", err
	}
	return runFolderPicker(cmd)
}

func runFolderPicker(cmd *exec.Cmd) (string, error) {
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail := strings.TrimSpace(string(exit.Stderr))
			if exit.ExitCode() == 1 && (runtime.GOOS == "linux" || runtime.GOOS == "darwin" && strings.Contains(detail, "(-128)")) {
				return "", nil
			}
			if detail != "" {
				return "", fmt.Errorf("folder picker: %s", detail)
			}
		}
		return "", fmt.Errorf("could not open folder picker: %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(output), "\n"), "\r"), nil
}

// PickAddonFile allows canceling without leaving the import input unusable.
func PickAddonFile() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e", `POSIX path of (choose file with prompt "Select an addon JSON file")`)
	case "windows":
		cmd = exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $picker = New-Object System.Windows.Forms.OpenFileDialog; $picker.Filter = 'JSON files (*.json)|*.json'; if ($picker.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; [Console]::Write($picker.FileName) }; $picker.Dispose()`)
	default:
		if path, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.Command(path, "--file-selection", "--title=Select an addon", "--file-filter=JSON files | *.json")
		} else if path, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.Command(path, "--getopenfilename", ".", "*.json", "--title", "Select an addon")
		} else {
			return "", fmt.Errorf("no file picker available; type the JSON file path")
		}
	}
	return runFolderPicker(cmd)
}
