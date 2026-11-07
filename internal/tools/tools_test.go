package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rian/internal/git"
	"rian/internal/project"
)

func TestURLsAndRepositoryOverride(t *testing.T) {
	for _, value := range []string{"https://example.com", "http://localhost:3000", "https://example.com/a?x=1", ""} {
		if err := ValidateURL(value); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
	for _, value := range []string{"javascript:alert(1)", "file:///tmp/a", "https://", "example.com"} {
		if err := ValidateURL(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	value, err := GitHubURL(project.Project{GitHub: "override/repo"}, git.Status{GitHub: "detected/repo"})
	if err != nil || value != "https://github.com/override/repo" {
		t.Fatalf("%s %v", value, err)
	}
	if _, err := GitHubURL(project.Project{}, git.Status{}); err == nil {
		t.Fatal("missing remote accepted")
	}
}

func TestMacApplicationsInSystemAndUserDirectories(t *testing.T) {
	root := t.TempDir()
	system, user := filepath.Join(root, "Applications"), filepath.Join(root, "User Applications")
	app := filepath.Join(user, "Visual Studio Code.app")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	if path := applicationPath("Visual Studio Code", []string{system, user}); path != app {
		t.Fatalf("application path: %q", path)
	}
	if path := applicationPath("Missing", []string{system, user}); path != "" {
		t.Fatalf("missing app accepted: %q", path)
	}
}

func TestShellRunsInSelectedDirectory(t *testing.T) {
	dir := t.TempDir()
	cmd := Shell(dir, "echo rian-command-ok")
	background(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "rian-command-ok") || cmd.Dir != dir {
		t.Fatalf("%s %v", output, err)
	}
}
