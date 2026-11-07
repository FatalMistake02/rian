package project

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFuzzyLookup(t *testing.T) {
	projects := []Project{{ID: "one", Name: "Cardly", Path: "/dev/cardly"}, {ID: "two", Name: "Card Scanner Test", Path: "/dev/scanner"}, {ID: "three", Name: "Drift", Path: "/dev/drift"}}
	if matches := Filter(projects, "car"); len(matches) != 2 || matches[0].Name != "Cardly" {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	for _, query := range []string{"Cardly", "CARDLY", "one", "cdly"} {
		p, err := Lookup(projects, query)
		if err != nil || p.ID != "one" {
			t.Fatalf("%q: %+v %v", query, p, err)
		}
	}
	if _, err := Lookup(projects, "car"); err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "two") {
		t.Fatalf("expected ambiguity with IDs, got %v", err)
	}
	if _, err := Lookup(projects, "xyz"); err == nil {
		t.Fatal("expected no match")
	}
	if _, err := Lookup(projects[:1], ""); err == nil {
		t.Fatal("empty query must not select a project")
	}
	if Score("ré", "Résumé") < 0 || Score("ab", "a"+strings.Repeat("x", 500)+"b") < 0 {
		t.Fatal("subsequence matches must not be lost")
	}
	if Score("drf", "Drift") < 0 || Score("frd", "Drift") >= 0 {
		t.Fatal("fuzzy letters must stay in order")
	}
	projects[1].Name = "Cardly"
	if _, err := Lookup(projects, "Cardly"); err == nil {
		t.Fatal("duplicate display names require ID")
	}
}

func TestOrderingAndCommands(t *testing.T) {
	now := time.Now()
	projects := []Project{{ID: "recent", Name: "Zed", LastUsed: now}, {ID: "fav", Name: "Alpha", Favorite: true}, {ID: "old", Name: "Beta"}}
	Sort(projects)
	if projects[0].ID != "fav" || projects[1].ID != "recent" {
		t.Fatalf("wrong order: %+v", projects)
	}
	p := Project{AutoCommands: map[string]string{"dev": "npm run dev", "test": "npm test"}, Commands: map[string]string{"dev": "pnpm dev"}, Tools: map[string]bool{"opencode": false}}
	if p.AllCommands()["dev"] != "pnpm dev" || p.AllCommands()["test"] != "npm test" {
		t.Fatal("custom commands should override detection")
	}
	if p.ToolEnabled("opencode") || !p.ToolEnabled("vscode") {
		t.Fatal("unexpected tool settings")
	}
	p.AllCommands()["test"] = "changed"
	if p.AutoCommands["test"] != "npm test" {
		t.Fatal("merged command map must not alias the project")
	}
}

func TestCanonicalPath(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, err := CanonicalPath(filepath.Join(dir, "."))
	if err != nil || !SamePath(path, resolved) {
		t.Fatalf("%q %v", path, err)
	}
	if _, err := CanonicalPath(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
}
