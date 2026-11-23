package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectOnlyUpdatesOwningGlobalManager(t *testing.T) {
	global := filepath.Join(t.TempDir(), "global")
	root := filepath.Join(global, "node_modules", "@fatalmistake02", "rian")
	exe := filepath.Join(root, "dist", "binaries", "linux-x64", "rian")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"npm", "bun", "local"} {
		t.Run(method, func(t *testing.T) {
			query := func(name string, args ...string) (string, error) {
				if method == "npm" && name == "npm" {
					return filepath.Join(global, "node_modules"), nil
				}
				if method == "bun" && name == "bun" {
					return global + " node_modules (1 installed)\n└── @fatalmistake02/rian", nil
				}
				return "", fmt.Errorf("unavailable")
			}
			want := method
			if method == "local" {
				want = "package"
			}
			if got := detect(exe, query); got.Method != want {
				t.Fatalf("detected %s, want %s", got.Method, want)
			}
		})
	}
}

func TestDetectHomebrewAndRefuseDifferentPrefix(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "Cellar", "rian", "1.0.0")
	exe := filepath.Join(prefix, "bin", "rian")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	query := func(name string, args ...string) (string, error) { return prefix, nil }
	if got := detect(exe, query); got.Method != "brew" {
		t.Fatalf("detected %s", got.Method)
	}
	query = func(name string, args ...string) (string, error) { return t.TempDir(), nil }
	if got := detect(exe, query); got.Method != "package" {
		t.Fatalf("detected %s", got.Method)
	}
}

func TestUnknownInstallCannotSelfUpdate(t *testing.T) {
	for _, method := range []string{"source", "package", ""} {
		_, err := (Installation{Method: method}).command("1.0.0")
		if err == nil || !strings.Contains(err.Error(), "original installation method") {
			t.Fatalf("method %q: %v", method, err)
		}
	}
}

func TestAliasedPackageIsNeverTreatedAsStandalone(t *testing.T) {
	previous := distribution
	distribution = "standalone"
	defer func() { distribution = previous }()
	exe := filepath.Join(t.TempDir(), "node_modules", "alias", "dist", "binaries", "linux-x64", "rian")
	query := func(string, ...string) (string, error) { return "", fmt.Errorf("not global") }
	if got := detect(exe, query); got.Method != "package" {
		t.Fatalf("aliased package detected as %s", got.Method)
	}
}
