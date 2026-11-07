// Package project contains the persistent project model and lookup rules.
package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Project struct {
	Notes        string            `json:"notes,omitempty"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Favorite     bool              `json:"favorite,omitempty"`
	GitHub       string            `json:"github,omitempty"`
	Tools        map[string]bool   `json:"tools,omitempty"`
	Commands     map[string]string `json:"commands,omitempty"`
	AutoCommands map[string]string `json:"auto_commands,omitempty"`
	Stack        []string          `json:"stack,omitempty"`
	URLs         map[string]string `json:"urls,omitempty"`
	AddedAt      time.Time         `json:"added_at"`
	LastUsed     time.Time         `json:"last_used,omitempty"`
}

func New(name, path string) Project {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// A timestamp remains sufficient for a local entry if the OS RNG fails.
		return Project{ID: fmt.Sprintf("%x", time.Now().UnixNano()), Name: name, Path: path, AddedAt: time.Now()}
	}
	return Project{ID: hex.EncodeToString(b), Name: name, Path: path, AddedAt: time.Now()}
}

func (p Project) ToolEnabled(name string) bool {
	enabled, set := p.Tools[name]
	return !set || enabled
}

func (p Project) AllCommands() map[string]string {
	result := make(map[string]string, len(p.AutoCommands)+len(p.Commands))
	for name, command := range p.AutoCommands {
		result[name] = command
	}
	for name, command := range p.Commands {
		result[name] = command
	}
	return result
}

func CanonicalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(path, "~"), "/\\"))
	}
	if path == "" {
		return "", fmt.Errorf("enter a project directory")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func Sort(projects []Project) {
	sort.SliceStable(projects, func(i, j int) bool {
		a, b := projects[i], projects[j]
		if a.Favorite != b.Favorite {
			return a.Favorite
		}
		if !a.LastUsed.Equal(b.LastUsed) {
			return a.LastUsed.After(b.LastUsed)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// Score performs Unicode-aware subsequence matching. Consecutive letters and
// word boundaries rank ahead of scattered matches. A negative score is no match.
func Score(query, value string) int {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	v := []rune(strings.ToLower(value))
	if len(q) == 0 {
		return 0
	}
	qi, score, previous := 0, 0, -2
	for i, r := range v {
		if r != q[qi] {
			continue
		}
		score += 10
		if i == previous+1 {
			score += 12
		}
		if i == 0 || !unicode.IsLetter(v[i-1]) && !unicode.IsDigit(v[i-1]) {
			score += 8
		}
		score -= i - previous - 1
		previous = i
		qi++
		if qi == len(q) {
			return max(0, score-(len(v)-len(q))/3)
		}
	}
	return -1
}

func Filter(projects []Project, query string) []Project {
	result := make([]Project, 0, len(projects))
	pathQuery := strings.ToLower(strings.TrimSpace(query))
	for _, p := range projects {
		if Score(query, p.Name) >= 0 || strings.Contains(strings.ToLower(p.Path), pathQuery) {
			result = append(result, p)
		}
	}
	if strings.TrimSpace(query) != "" {
		sort.SliceStable(result, func(i, j int) bool { return Score(query, result[i].Name) > Score(query, result[j].Name) })
	}
	return result
}

func Lookup(projects []Project, query string) (Project, error) {
	if strings.TrimSpace(query) == "" {
		return Project{}, fmt.Errorf("enter a project name or ID")
	}
	var exact []Project
	for _, p := range projects {
		if p.ID == query {
			return p, nil
		}
		if strings.EqualFold(p.Name, query) {
			exact = append(exact, p)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = Filter(projects, query)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return Project{}, fmt.Errorf("no project matches %q", query)
	}
	var names []string
	for _, p := range matches {
		names = append(names, fmt.Sprintf("  %s  [%s]  %s", p.Name, p.ID, p.Path))
	}
	return Project{}, fmt.Errorf("%q is ambiguous; use a project ID:\n%s", query, strings.Join(names, "\n"))
}
