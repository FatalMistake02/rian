package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestOfferOnlyOnceForEitherAnswer(t *testing.T) {
	for _, answer := range []string{"yes\n", "n\n", "\n", ""} {
		t.Run(fmt.Sprintf("%q", answer), func(t *testing.T) {
			dir := t.TempDir()
			var out bytes.Buffer
			calls := 0
			register := func() error { calls++; return nil }
			if err := offer(dir, strings.NewReader(answer), &out, register); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "[y/N]") {
				t.Fatal("offer missing")
			}
			out.Reset()
			if err := offer(dir, strings.NewReader("yes\n"), &out, register); err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatal("asked again")
			}
			expected := 0
			if answer == "yes\n" {
				expected = 1
			}
			if calls != expected {
				t.Fatalf("registration called %d times", calls)
			}
		})
	}
}
func TestConcurrentOffersAndFailureDoNotRepeat(t *testing.T) {
	dir := t.TempDir()
	var prompts atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out bytes.Buffer
			err := offer(dir, strings.NewReader("yes\n"), &out, func() error { prompts.Add(1); return fmt.Errorf("fixture failure") })
			if err != nil && !strings.Contains(err.Error(), "fixture failure") {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if prompts.Load() != 1 {
		t.Fatalf("offered %d times", prompts.Load())
	}
}
func TestNonInteractiveDoesNotConsumeOffer(t *testing.T) {
	dir := t.TempDir()
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var out bytes.Buffer
	if err := Offer(dir, input, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-offer.json")); !os.IsNotExist(err) {
		t.Fatal("noninteractive install consumed offer")
	}
	if out.Len() != 0 {
		t.Fatal("noninteractive prompt")
	}
}
func TestLinuxLauncher(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data with spaces")
	binary := filepath.Join(home, "Rian $ tools", "100%", "rian")
	if err := write(binary, "test executable", 0755); err != nil {
		t.Fatal(err)
	}
	if err := Register("linux", home, binary, data); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(data, "applications", "rian.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if validator, err := exec.LookPath("desktop-file-validate"); err == nil {
		if output, err := exec.Command(validator, filepath.Join(data, "applications", "rian.desktop")).CombinedOutput(); err != nil {
			t.Fatalf("invalid desktop entry: %s: %v", output, err)
		}
	}
	text := string(body)
	for _, expected := range []string{"Name=Rian\n", "Terminal=true\n", "Exec=\"", "100%%", "Categories=Development;Utility;"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
	if strings.Contains(text, "Exec=rian\n") {
		t.Fatal("launcher depends on shell PATH")
	}
	if err := Register("linux", home, "bad\npath", data); err == nil {
		t.Fatal("accepted newline in executable path")
	}
}
func TestMacLauncherUsesTerminal(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(home, "tools", "rian")
	if err := write(binary, "test executable", 0755); err != nil {
		t.Fatal(err)
	}
	if err := Register("darwin", home, binary, ""); err != nil {
		t.Fatal(err)
	}
	contents := filepath.Join(home, "Applications", "Rian.app", "Contents")
	body, err := os.ReadFile(filepath.Join(contents, "MacOS", "rian"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "osascript") || !strings.Contains(string(body), "Terminal") || !strings.Contains(string(body), "exec") {
		t.Fatal("mac app does not launch a terminal")
	}
	stat, err := os.Stat(filepath.Join(contents, "MacOS", "rian"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && stat.Mode()&0111 == 0 {
		t.Fatal("mac launcher not executable")
	}
	if _, err := os.Stat(filepath.Join(contents, "Info.plist")); err != nil {
		t.Fatal(err)
	}
}

func TestLauncherKeepsNativeTargetWhenAliasChanges(t *testing.T) {
	home := t.TempDir()
	native, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	native, err = filepath.EvalSymlinks(native)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "rian")
	if err := os.Symlink(native, alias); err != nil {
		t.Skipf("executable symlinks unavailable: %v", err)
	}
	data := filepath.Join(home, "share")
	if err := Register("linux", home, alias, data); err != nil {
		t.Fatal(err)
	}
	if err := Register("darwin", home, alias, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(home, "rian.cjs")
	if err := write(shim, "#!/usr/bin/env node\nprocess.exit(97);\n", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shim, alias); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(data, "applications", "rian.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Exec="+desktopQuote(native)+"\n") {
		t.Fatalf("desktop shortcut did not retain native target: %s", body)
	}
	body, err = os.ReadFile(filepath.Join(home, "Applications", "Rian.app", "Contents", "MacOS", "rian"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), alias) {
		t.Fatalf("macOS launcher retained mutable alias: %s", body)
	}
	cmd := exec.Command(native, "-test.run=^TestDesktopExecutableHelper$")
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir(), "RIAN_DESKTOP_TEST_HELPER=1")
	if output, err := cmd.CombinedOutput(); err != nil || string(output) != "rian native-test\n" {
		t.Fatalf("native shortcut target needs a tool from PATH: %q (%v)", output, err)
	}
}

func TestDesktopExecutableHelper(t *testing.T) {
	if os.Getenv("RIAN_DESKTOP_TEST_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "rian native-test")
	os.Exit(0)
}

func TestUnavailableExecutablePreservesLauncher(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "share")
	launcher := filepath.Join(data, "applications", "rian.desktop")
	const original = "existing launcher\n"
	if err := write(launcher, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Register("linux", home, filepath.Join(home, "missing"), data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing executable error was lost: %v", err)
	}
	if err := Register("linux", home, home, data); err == nil {
		t.Fatal("directory accepted as an executable")
	}
	body, err := os.ReadFile(launcher)
	if err != nil || string(body) != original {
		t.Fatalf("failed registration changed the launcher: %q (%v)", body, err)
	}
}
