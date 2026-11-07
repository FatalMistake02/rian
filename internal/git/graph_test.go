package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphTracksLocalOriginAndMergeTopology(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
		return strings.TrimSpace(string(output))
	}
	commit := func(file, subject string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(subject), 0600); err != nil {
			t.Fatal(err)
		}
		run("add", "--", file)
		run("commit", "-m", subject)
		return run("rev-parse", "HEAD")
	}
	run("init", "-b", "main")
	run("config", "user.name", "Rian Test")
	run("config", "user.email", "rian@example.test")
	run("config", "commit.gpgsign", "false")
	base := commit("base", "Shared base")
	run("switch", "-c", "feature")
	feature := commit("feature", "Feature work")
	run("switch", "main")
	local := commit("main", "Local main work")
	run("branch", "same-tip")
	run("tag", "-a", "v1", "-m", "Tag", "HEAD")
	run("switch", "-c", "remote-work", base)
	remote := commit("remote", "Origin work")
	run("update-ref", "refs/remotes/origin/main", remote)
	run("switch", "main")
	run("branch", "-D", "remote-work")
	run("remote", "add", "origin", "https://github.com/example/repo.git")
	run("config", "branch.main.remote", "origin")
	run("config", "branch.main.merge", "refs/heads/main")
	s := Inspect(dir)
	if s.Head != local || s.Upstream != "origin/main" || s.Ahead != 1 || s.Behind != 1 {
		t.Fatalf("tracking state: %+v", s)
	}
	hashes := func(rows []GraphLine) map[string]bool {
		result := map[string]bool{}
		for _, row := range rows {
			if row.Hash != "" {
				result[row.Hash] = true
			}
		}
		return result
	}
	current, all := hashes(s.CurrentGraphLines), hashes(s.GraphLines)
	if !current[base] || !current[local] || !current[remote] || current[feature] || !all[feature] {
		t.Fatal("current graph did not isolate local and origin histories")
	}
	pathConnectors := 0
	for _, row := range s.GraphLines {
		if row.Hash == local || row.Hash == base {
			if !row.CurrentBranch {
				t.Fatal("current first-parent commit is not colored")
			}
		} else if row.Hash != "" && row.CurrentBranch {
			t.Fatal("another branch commit inherited the current branch color")
		}
		for _, cell := range row.Cells {
			if cell.CurrentBranch && cell.Glyph != '●' && cell.Glyph != ' ' {
				pathConnectors++
			}
		}
	}
	if pathConnectors == 0 {
		t.Fatal("current branch color does not continue between commits")
	}
	localRefs, remoteRefs, currentRef, tag := false, false, false, false
	colors := map[int]bool{}
	for _, row := range s.GraphLines {
		for _, cell := range row.Cells {
			if cell.Color >= 0 {
				colors[cell.Color] = true
			}
		}
		for _, ref := range row.Refs {
			localRefs = localRefs || ref.Kind == "local" && ref.Name == "same-tip"
			remoteRefs = remoteRefs || ref.Kind == "remote" && ref.Name == "origin/main" && row.Hash == remote
			currentRef = currentRef || ref.Current && ref.Name == "main" && row.Current && row.Hash == local
			tag = tag || ref.Kind == "tag" && ref.Name == "v1" && row.Hash == local
		}
	}
	if !localRefs || !remoteRefs || !currentRef || !tag || len(colors) < 2 {
		t.Fatalf("refs local=%v remote=%v current=%v tag=%v colors=%v rows=%+v", localRefs, remoteRefs, currentRef, tag, colors, s.GraphLines)
	}
	run("branch", "--unset-upstream")
	s = Inspect(dir)
	if s.Upstream != "origin/main" || s.Ahead != 1 || s.Behind != 1 || !hashes(s.CurrentGraphLines)[remote] {
		t.Fatal("matching origin fallback was not included")
	}
	run("merge", "--no-ff", "feature", "-m", "Merge feature")
	s = Inspect(dir)
	for _, row := range s.GraphLines {
		if row.Hash == feature && row.CurrentBranch {
			t.Fatal("merged side branch was colored as the main branch")
		}
		if row.Hash == local && !row.CurrentBranch {
			t.Fatal("merge interrupted the current branch color")
		}
	}
	if !hashes(s.CurrentGraphLines)[feature] {
		t.Fatal("current graph lost merged ancestry")
	}
	connectors := false
	for _, row := range s.CurrentGraphLines {
		for _, cell := range row.Cells {
			connectors = connectors || cell.Glyph == '╱' || cell.Glyph == '╲'
		}
	}
	if !connectors {
		t.Fatal("merge topology has no Unicode connectors")
	}
	run("switch", "--detach", base)
	s = Inspect(dir)
	if len(hashes(s.CurrentGraphLines)) != 1 || !hashes(s.CurrentGraphLines)[base] {
		t.Fatal("detached view includes unrelated branches")
	}
}
