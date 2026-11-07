package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchLauncherPassesDirectoryLiterally(t *testing.T) {
	dir := t.TempDir()
	launcher := filepath.Join(dir, "rian-test-launcher.cmd")
	if err := os.WriteFile(launcher, []byte("@echo off\r\necho \"%~1\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	argument := filepath.Join(dir, "A & B!")
	cmd, err := executable(launcher, argument)
	if err != nil {
		t.Fatal(err)
	}
	background(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), argument) {
		t.Fatalf("batch argument changed: %q %v", output, err)
	}
	if _, err := executable(launcher, filepath.Join(dir, "%PATH%")); err == nil {
		t.Fatal("cmd expansion characters must be rejected")
	}
}
