package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rian/internal/config"
)

func TestCLIProjectLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RIAN_CONFIG_HOME", filepath.Join(root, "settings"))
	dir := filepath.Join(root, "Sample Project")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"test":"echo project-ok"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	call := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		err := Run(args, in, &out, "test")
		return out.String(), err
	}
	if _, err := call("add", dir); err == nil {
		t.Fatal("noninteractive add must require --yes")
	}
	if _, err := call("add", dir, "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := call("add", dir, "--yes"); err == nil {
		t.Fatal("duplicate add should fail")
	}
	output, err := call("list")
	if err != nil || !strings.Contains(output, "Sample Project") {
		t.Fatalf("list: %s %v", output, err)
	}
	output, err = call("list", "--json")
	if err != nil || !json.Valid([]byte(output)) {
		t.Fatalf("JSON: %s %v", output, err)
	}
	if _, err := call("run", "sample", "unknown"); err == nil || !strings.Contains(err.Error(), "available: test") {
		t.Fatalf("missing command: %v", err)
	}
	path, _ := config.DefaultPath()
	store := config.New(path)
	if _, err := store.Update(func(cfg *config.Config) error {
		cfg.Projects[0].Commands = map[string]string{"smoke": "echo project-ok"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	output, err = call("run", "Sample Project", "smoke")
	if err != nil || !strings.Contains(output, "project-ok") {
		t.Fatalf("run: %s %v", output, err)
	}
	cfg, _ := store.Load()
	if cfg.Projects[0].LastUsed.IsZero() {
		t.Fatal("CLI did not record use")
	}
	if _, err := call("remove", "sample", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		t.Fatal("removal touched project files")
	}
}

func TestHelpDoesNotNeedConfig(t *testing.T) {
	t.Setenv("RIAN_CONFIG_HOME", filepath.Join(t.TempDir(), "missing"))
	var out bytes.Buffer
	if err := Run([]string{"--help"}, os.Stdin, &out, "test"); err != nil || !strings.Contains(out.String(), "rian add") {
		t.Fatalf("%s %v", out.String(), err)
	}
}
