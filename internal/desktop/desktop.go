// Package desktop registers a per-user launcher and records a one-time offer.
package desktop

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

// Offer only consumes the offer when a real terminal is available. The marker
// is claimed before prompting so concurrent installs cannot prompt twice.
func Offer(configDir string, in *os.File, out io.Writer) error {
	if in == nil || !term.IsTerminal(in.Fd()) {
		return nil
	}
	return offer(configDir, in, out, func() error {
		binary, err := os.Executable()
		if err != nil {
			return err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return Register(runtime.GOOS, home, binary, os.Getenv("XDG_DATA_HOME"))
	})
}
func offer(configDir string, in io.Reader, out io.Writer, register func() error) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	marker, err := os.OpenFile(filepath.Join(configDir, "app-offer.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = marker.WriteString("{\"asked\":true}\n")
	closeErr := marker.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprint(out, "Add Rian to your apps so you can search for it and open it in a terminal? [y/N] ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(out, "Skipped. Rian won't ask again.")
		return nil
	}
	if err := register(); err != nil {
		return fmt.Errorf("could not add Rian to apps: %w", err)
	}
	fmt.Fprintln(out, "Rian added to your apps.")
	return nil
}

// OfferInstallation uses the controlling terminal when npm hides lifecycle
// stdin/output. If no terminal exists, the first interactive launch offers it.
func OfferInstallation(configDir string) error {
	if _, err := os.Stat(filepath.Join(configDir, "app-offer.json")); err == nil {
		return nil
	}
	if term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) {
		return Offer(configDir, os.Stdin, os.Stdout)
	}
	in, out, closeTerminal, err := controllingTerminal()
	if err != nil {
		return nil
	}
	defer closeTerminal()
	return Offer(configDir, in, out)
}
func write(path, body string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), mode)
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func desktopQuote(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$").Replace(value)
	// Desktop-entry string escaping is applied before Exec argument parsing.
	return "\"" + strings.ReplaceAll(value, "\\", "\\\\") + "\""
}
func Register(platform, home, binary, dataHome string) error {
	binary, err := filepath.Abs(binary)
	if err != nil {
		return err
	}
	if strings.ContainsAny(binary+home, "\x00\r\n") {
		return fmt.Errorf("launcher paths must not contain control characters")
	}
	// Pin shortcuts to the native executable rather than an alias that could
	// later be replaced with a launcher requiring a different environment.
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return fmt.Errorf("resolve Rian executable: %w", err)
	}
	if strings.ContainsAny(binary, "\x00\r\n") {
		return fmt.Errorf("launcher paths must not contain control characters")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return fmt.Errorf("read Rian executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Rian executable must be a regular file: %s", binary)
	}
	switch platform {
	case "linux":
		if dataHome == "" || !filepath.IsAbs(dataHome) {
			dataHome = filepath.Join(home, ".local", "share")
		}
		body := "[Desktop Entry]\nType=Application\nName=Rian\nComment=Open Rian in a terminal\nExec=" + desktopQuote(binary) + "\nTerminal=true\nIcon=utilities-terminal\nCategories=Development;Utility;\nKeywords=rian;projects;terminal;\nStartupNotify=false\n"
		return write(filepath.Join(dataHome, "applications", "rian.desktop"), body, 0644)
	case "darwin":
		contents := filepath.Join(home, "Applications", "Rian.app", "Contents")
		plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleName</key><string>Rian</string><key>CFBundleDisplayName</key><string>Rian</string><key>CFBundleIdentifier</key><string>dev.rian.launcher</string><key>CFBundleExecutable</key><string>rian</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleVersion</key><string>1</string></dict></plist>`
		if err := write(filepath.Join(contents, "Info.plist"), plist, 0644); err != nil {
			return err
		}
		script := "cd " + shellQuote(home) + " && exec " + shellQuote(binary)
		apple := "tell application \"Terminal\"\nactivate\ndo script " + strconv.Quote(script) + "\nend tell"
		return write(filepath.Join(contents, "MacOS", "rian"), "#!/bin/sh\nexec /usr/bin/osascript -e "+shellQuote(apple)+"\n", 0755)
	case "windows":
		apps := os.Getenv("APPDATA")
		if apps == "" {
			apps = filepath.Join(home, "AppData", "Roaming")
		}
		destination := filepath.Join(apps, "Microsoft", "Windows", "Start Menu", "Programs", "Rian.lnk")
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		// Values arrive through environment variables, never PowerShell source.
		script := `$ErrorActionPreference='Stop'; $shell=New-Object -ComObject WScript.Shell; $link=$shell.CreateShortcut($env:RIAN_APP_SHORTCUT); $link.TargetPath=$env:RIAN_APP_BINARY; $link.WorkingDirectory=$env:RIAN_APP_HOME; $link.Description='Open Rian in a terminal'; $link.Save()`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "RIAN_APP_SHORTCUT="+destination, "RIAN_APP_BINARY="+binary, "RIAN_APP_HOME="+home)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("create Start menu shortcut: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	default:
		return fmt.Errorf("app registration is unavailable on %s", platform)
	}
}
