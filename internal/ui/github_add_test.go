package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/git"
)

// The copied test executable acts as a native gh fixture on every CI platform.
// Dispatch before testing parses gh's arguments as test flags.
func TestMain(m *testing.M) {
	if os.Getenv("RIAN_UI_TEST_GH_HELPER") == "1" && strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "gh" {
		os.Exit(runGitHubFixture(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runGitHubFixture(args []string) int {
	if os.Getenv("RIAN_UI_TEST_GH_FAIL") == "1" {
		fmt.Fprintln(os.Stderr, "repository unavailable")
		return 1
	}
	if len(args) < 2 {
		return 2
	}
	switch args[0] + " " + args[1] {
	case "api user":
		fmt.Fprintln(os.Stdout, "tester")
		return 0
	case "repo fork", "repo create":
		return 0
	case "repo clone":
		if len(args) != 4 {
			return 2
		}
		for _, gitArgs := range [][]string{
			{"init", "-q", args[3]},
			{"-C", args[3], "remote", "add", "origin", "https://github.com/" + args[2] + ".git"},
		} {
			cmd := exec.Command("git", gitArgs...)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return 1
			}
		}
		return 0
	default:
		return 2
	}
}

func installGitHubFixture(t *testing.T, fail bool) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RIAN_UI_TEST_GH_HELPER", "1")
	if fail {
		t.Setenv("RIAN_UI_TEST_GH_FAIL", "1")
	} else {
		t.Setenv("RIAN_UI_TEST_GH_FAIL", "0")
	}
	resolved, err := exec.LookPath("gh")
	if err != nil {
		t.Fatalf("native gh fixture was not selected: %q, %v", resolved, err)
	}
	selected, err := os.Stat(resolved)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.Stat(filepath.Join(bin, name))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(selected, expected) {
		t.Fatalf("native gh fixture was not selected: %q", resolved)
	}
}

func TestGitHubAddNavigationAndPickerCancellation(t *testing.T) {
	m := fixture(t)
	m.startAdd()
	m.modalKey(press('g', "g"))
	if m.mode != githubAdd {
		t.Fatal("GitHub choice missing")
	}
	m.githubAddKey(press(tea.KeyDown, ""))
	m.githubAddKey(press(tea.KeyEnter, ""))
	m.query.SetValue("https://github.com/example/demo.git")
	m.githubAddKey(press(tea.KeyEnter, ""))
	if m.githubStage != 2 || m.githubRepo != "example/demo" {
		t.Fatal("repository not normalized")
	}
	m.Update(pickedFolderMsg{})
	destination := t.TempDir()
	resolved, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	m.query.SetValue(destination)
	m.githubAddKey(press(tea.KeyEnter, ""))
	if m.githubStage != 3 || m.githubPath != resolved {
		t.Fatalf("clone remote choice missing: stage=%d, path=%q, want=%q, error=%q", m.githubStage, m.githubPath, resolved, m.formError)
	}
	m.githubAddKey(press(tea.KeyEscape, ""))
	if m.githubStage != 2 {
		t.Fatal("back lost location step")
	}
}

func TestGitHubDestinationResolvesSymlinkParents(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			name := map[bool]string{false: "new", true: "existing"}[existing]
			if existing {
				if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			m := fixture(t)
			m.mode, m.githubKind, m.githubStage = githubAdd, "clone", 2
			m.githubChooseLocation(filepath.Join(alias, name))
			if m.githubStage != 3 || m.githubPath != filepath.Join(resolved, name) {
				t.Fatalf("symlink parent lost clone confirmation: stage=%d, path=%q, error=%q", m.githubStage, m.githubPath, m.formError)
			}
		})
	}
}

func TestGitHubDestinationDoesNotOverwrite(t *testing.T) {
	parent := t.TempDir()
	os.Mkdir(filepath.Join(parent, "demo"), 0700)
	if _, err := githubDestination(parent); err == nil {
		t.Fatal("existing directory accepted")
	}
}

func TestGitHubImportCompletesInsideOutput(t *testing.T) {
	for _, kind := range []string{"clone", "clone-no-remote", "fork"} {
		t.Run(kind, func(t *testing.T) {
			installGitHubFixture(t, false)
			m := fixture(t)
			t.Cleanup(m.manager.Close)
			m.githubKind, m.githubRepo = kind, "example/demo"
			if kind == "clone-no-remote" {
				m.githubKind = "clone"
			}
			m.githubPath = t.TempDir()
			cmd := m.beginGitHubImport(kind != "clone-no-remote")
			deadline := time.Now().Add(5 * time.Second)
			for m.mode == gitOutput && time.Now().Before(deadline) {
				if cmd != nil {
					msg := cmd()
					if run, ok := msg.(gitRunMsg); ok {
						cmd = m.gitProgress(run)
					} else {
						t.Fatalf("unexpected %T", msg)
					}
				} else {
					time.Sleep(10 * time.Millisecond)
					cmd = m.gitProgress(gitRunMsg{job: m.gitJobID, state: m.manager.Snapshot(m.gitJobID)})
				}
				if !m.gitRunning && m.mode == gitOutput {
					t.Fatalf("import failed: %s", m.gitError)
				}
			}
			if m.mode != add || cmd == nil {
				t.Fatal("import did not proceed to project inspection")
			}
			msg, ok := cmd().(inspectedMsg)
			if !ok || msg.err != nil {
				t.Fatalf("inspection: %#v", msg)
			}
			m.Update(msg)
			if m.mode != addConfirm {
				t.Fatal("project confirmation missing")
			}
			status := git.Inspect(m.githubPath)
			remotes := map[string]string{}
			for _, r := range status.Remotes {
				remotes[r.Name] = r.URL
			}
			if kind == "clone-no-remote" && len(remotes) != 0 {
				t.Fatal("clone remote retained")
			}
			if kind == "fork" && (remotes["origin"] != "https://github.com/tester/demo.git" || remotes["upstream"] != "https://github.com/example/demo.git") {
				t.Fatalf("fork remotes: %v", remotes)
			}
		})
	}
}

func TestGitHubImportFailureStaysOpen(t *testing.T) {
	installGitHubFixture(t, true)
	m := fixture(t)
	t.Cleanup(m.manager.Close)
	m.githubKind, m.githubRepo, m.githubPath = "clone", "example/demo", filepath.Join(t.TempDir(), "demo")
	cmd := m.beginGitHubImport(true)
	cmd = m.gitProgress(cmd().(gitRunMsg))
	deadline := time.Now().Add(3 * time.Second)
	for m.gitRunning && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		cmd = m.gitProgress(gitRunMsg{job: m.gitJobID, state: m.manager.Snapshot(m.gitJobID)})
	}
	_ = cmd
	if m.mode != gitOutput || m.gitRunning || m.gitError == "" {
		t.Fatal("failure did not stay open")
	}
	m.modalKey(press(tea.KeyEscape, ""))
	if m.mode != home {
		t.Fatal("failed output could not be closed")
	}
}
