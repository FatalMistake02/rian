// Package git reads local repository state. Inspect never fetches the network.
package git

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type GraphCell struct {
	Glyph         rune
	Color         int
	CurrentBranch bool
}
type GraphRef struct {
	Name, Kind string
	Current    bool
}
type GraphLine struct {
	Text, Hash         string
	ShortHash, Subject string
	Cells              []GraphCell
	Refs               []GraphRef
	Current            bool
	CurrentBranch      bool
}

type Status struct {
	Remotes                            []Remote
	Tracking                           string
	RemoteName                         string
	Conflicted                         bool
	Attached                           bool
	Repository                         bool
	Branch                             string
	Modified, Untracked, Ahead, Behind int
	Staged                             int
	Branches                           []string
	Graph                              string
	GraphLines                         []GraphLine
	CurrentGraphLines                  []GraphLine
	Head, Upstream                     string
	upstreamRef                        string
	Remote, GitHub, Commit             string
	CommitTime                         time.Time
	Error                              string
}

type Remote struct{ Name, URL string }

func (s Status) Dirty() bool { return s.Modified+s.Untracked > 0 }

// GitHubRepo accepts SSH, HTTPS, git protocol and ssh:// remotes, but only
// github.com. Strict owner/repo validation also makes overrides safe URLs.
func GitHubRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	var path string
	if strings.HasPrefix(remote, "git@github.com:") {
		path = strings.TrimPrefix(remote, "git@github.com:")
	} else {
		u, err := url.Parse(remote)
		if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
			return ""
		}
		if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git" {
			return ""
		}
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(strings.TrimRight(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !validPart(parts[0]) || !validPart(parts[1]) {
		return ""
	}
	return path
}

func validPart(part string) bool {
	if part == "" || part == "." || part == ".." {
		return false
	}
	for _, r := range part {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func NormalizeRepo(repo string) (string, error) {
	if repo == "" {
		return "", nil
	}
	if parsed := GitHubRepo(repo); parsed != "" {
		return parsed, nil
	}
	if parsed := GitHubRepo("https://github.com/" + repo); parsed != "" {
		return parsed, nil
	}
	return "", fmt.Errorf("use a GitHub owner/repository or a github.com URL")
}

func Inspect(path string) Status {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
		hideWindow(cmd)
		output, err := cmd.Output()
		if len(args) > 0 && args[0] == "status" {
			return string(output), err
		}
		return strings.TrimSpace(string(output)), err
	}
	var s Status
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		s.Error = "This directory is no longer available"
		return s
	}
	if _, err := exec.LookPath("git"); err != nil {
		s.Error = "Git is not installed"
		return s
	}
	root, err := run("rev-parse", "--is-inside-work-tree")
	if err != nil || root != "true" {
		return s
	}
	s.Repository = true
	branch, err := run("symbolic-ref", "--quiet", "--short", "HEAD")
	s.Attached = err == nil
	s.Tracking, _ = run("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		branch, _ = run("rev-parse", "--short", "HEAD")
		if branch != "" {
			branch = "detached · " + branch
		} else {
			branch = "unborn"
		}
	}
	s.Branch = branch
	branches, _ := run("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if branches != "" {
		s.Branches = strings.Split(branches, "\n")
	}
	readGraphs(&s, run)
	output, err := run("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		s.Error = "Unable to read Git status"
	} else {
		s.Modified, s.Untracked = ParsePorcelain(output)
		entries := strings.Split(output, "\x00")
		for i := 0; i < len(entries); i++ {
			entry := entries[i]
			if len(entry) < 3 {
				continue
			}
			if strings.HasPrefix(entry, "UU") || strings.HasPrefix(entry, "AA") || strings.HasPrefix(entry, "DD") || entry[0] == 'U' || entry[1] == 'U' {
				s.Conflicted = true
			}
			if entry[0] != ' ' && entry[0] != '?' {
				s.Staged++
			}
			if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
				i++
			}
		}
	}
	remotes, _ := run("remote")
	for _, name := range strings.Fields(remotes) {
		value, _ := run("remote", "get-url", name)
		s.Remotes = append(s.Remotes, Remote{name, value})
		if s.RemoteName == "" || name == "origin" {
			s.RemoteName, s.Remote = name, value
		}
	}
	if s.Tracking != "" {
		configured, _ := run("config", "--get", "branch."+s.Branch+".remote")
		for _, remote := range s.Remotes {
			if remote.Name == configured {
				s.RemoteName, s.Remote = remote.Name, remote.URL
			}
		}
	}

	s.GitHub = GitHubRepo(s.Remote)
	counts, err := run("rev-list", "--left-right", "--count", "HEAD..."+s.upstreamRef)
	if err == nil && s.upstreamRef != "" {
		parts := strings.Fields(counts)
		if len(parts) == 2 {
			s.Ahead, _ = strconv.Atoi(parts[0])
			s.Behind, _ = strconv.Atoi(parts[1])
		}
	}
	commit, err := run("log", "-1", "--format=%ct%n%s")
	if err == nil {
		parts := strings.SplitN(commit, "\n", 2)
		if len(parts) == 2 {
			seconds, _ := strconv.ParseInt(parts[0], 10, 64)
			s.CommitTime = time.Unix(seconds, 0)
			s.Commit = parts[1]
		}
	}
	if ctx.Err() != nil {
		s.Error = "Git status timed out"
	}
	return s
}

func ParsePorcelain(output string) (modified, untracked int) {
	entries := strings.Split(output, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 3 {
			continue
		}
		if entry[:2] == "??" {
			untracked++
		} else {
			modified++
			if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
				i++
			}
		}
	}
	return
}
