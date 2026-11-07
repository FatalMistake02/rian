package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubRepo(t *testing.T) {
	for _, remote := range []string{"git@github.com:FatalMistake02/Cardly.git", "https://github.com/FatalMistake02/Cardly.git", "ssh://git@github.com/FatalMistake02/Cardly.git", "git://github.com/FatalMistake02/Cardly", "https://github.com/FatalMistake02/Cardly/"} {
		if actual := GitHubRepo(remote); actual != "FatalMistake02/Cardly" {
			t.Errorf("%q → %q", remote, actual)
		}
	}
	for _, remote := range []string{"", "git@gitlab.com:owner/repo.git", "https://github.com.evil.test/a/b", "https://github.com/a/b/tree/main", "https://evil.test/github.com/a/b", "https://github.com/a/../b", "file:///a/b", "https://github.com/a/b%20x", "https://github.com/a/.."} {
		if actual := GitHubRepo(remote); actual != "" {
			t.Errorf("accepted %q as %q", remote, actual)
		}
	}
	if actual, err := NormalizeRepo("owner/repo"); err != nil || actual != "owner/repo" {
		t.Fatalf("override: %q %v", actual, err)
	}
}

func TestPorcelainRenamesAndWhitespace(t *testing.T) {
	modified, untracked := ParsePorcelain(" M file with spaces\x00R  new name\x00old name\x00?? new file\x00 D deleted\x00")
	if modified != 3 || untracked != 1 {
		t.Fatalf("got modified %d, untracked %d", modified, untracked)
	}
}

func TestInspectRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, data, err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Rian Test")
	run("config", "user.email", "rian@example.test")
	run("config", "commit.gpgsign", "false")
	if status := Inspect(dir); !status.Repository || status.Branch != "main" {
		t.Fatalf("unborn branch: %+v", status)
	}
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "--", "file.txt")
	run("commit", "-m", "Initial project")
	run("remote", "add", "origin", "git@github.com:owner/repo.git")
	if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	s := Inspect(dir)
	if !s.Repository || s.Branch != "main" || s.Modified != 1 || s.Untracked != 1 || s.GitHub != "owner/repo" || s.Commit != "Initial project" || s.CommitTime.IsZero() || !s.Dirty() {
		t.Fatalf("unexpected status: %+v", s)
	}
	if len(s.GraphLines) != 1 || len(s.GraphLines[0].Hash) != 40 || !strings.Contains(s.GraphLines[0].Text, "Initial project") {
		t.Fatal("graph is missing full commit identity")
	}
	run("checkout", "--detach")
	if s = Inspect(dir); s.Branch == "main" || s.Branch == "" {
		t.Fatalf("detached status: %+v", s)
	}
}

func TestInspectListsNamedRemotesAndTracking(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Rian Test")
	run("config", "user.email", "rian@example.test")
	run("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Initial")
	run("remote", "add", "origin", "https://example.com/origin.git")
	run("remote", "add", "upstream", "https://github.com/example/upstream.git")
	run("update-ref", "refs/remotes/upstream/main", "HEAD")
	run("branch", "--set-upstream-to=upstream/main", "main")
	status := Inspect(dir)
	if len(status.Remotes) != 2 || status.RemoteName != "upstream" || status.Tracking != "upstream/main" || !status.Attached || status.Head == "" {
		t.Fatalf("remote tracking lost: %+v", status)
	}
	run("checkout", "--detach")
	if Inspect(dir).Attached {
		t.Fatal("detached HEAD marked as pushable branch")
	}
}
