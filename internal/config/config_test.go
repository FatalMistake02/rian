package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"rian/internal/project"
)

func TestRoundTripAndRemoval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings", "config.json")
	store := New(path)
	cfg, err := store.Load()
	if err != nil || len(cfg.Projects) != 0 {
		t.Fatalf("empty load: %+v %v", cfg, err)
	}
	p := project.New("My project", dir)
	p.Commands = map[string]string{"dev": "npm run dev"}
	p.Tools = map[string]bool{"opencode": false}
	p.URLs = map[string]string{"local": "http://localhost:3000"}
	if _, err := store.Update(func(cfg *Config) error { return Add(cfg, p) }); err != nil {
		t.Fatal(err)
	}
	cfg, err = New(path).Load()
	if err != nil || len(cfg.Projects) != 1 || cfg.Projects[0].Commands["dev"] != "npm run dev" || cfg.Projects[0].ToolEnabled("opencode") {
		t.Fatalf("round trip: %+v %v", cfg, err)
	}
	if _, err := store.Update(func(cfg *Config) error { return Add(cfg, p) }); err == nil {
		t.Fatal("duplicate directory accepted")
	}
	if _, err := store.Update(func(cfg *Config) error { return Touch(cfg, p.ID) }); err != nil {
		t.Fatal(err)
	}
	cfg, _ = store.Load()
	if cfg.Projects[0].LastUsed.IsZero() {
		t.Fatal("recent timestamp missing")
	}
	if _, err := store.Update(func(cfg *Config) error { return Remove(cfg, p.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("removal deleted the actual directory")
	}
	cfg, _ = store.Load()
	if len(cfg.Projects) != 0 {
		t.Fatal("entry not removed")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".rian-*.tmp"))
	if len(files) != 0 {
		t.Fatalf("left temporary files: %v", files)
	}
}

func TestInvalidConfigPreserved(t *testing.T) {
	for _, data := range []string{"{broken", `{"version":99,"projects":[]}`, `{"version":1,"projects":[{"id":"x","name":"x","path":"relative"}]}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := New(path).Update(func(*Config) error { return nil }); err == nil {
				t.Fatal("invalid config accepted")
			}
			actual, _ := os.ReadFile(path)
			if string(actual) != data {
				t.Fatal("invalid configuration overwritten")
			}
		})
	}
}

func TestConcurrentStoresDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := project.New(fmt.Sprintf("project-%d", i), filepath.Join(dir, fmt.Sprint(i)))
			_, err := New(path).Update(func(cfg *Config) error { return Add(cfg, p) })
			if err != nil {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	cfg, err := New(path).Load()
	if err != nil || len(cfg.Projects) != 8 {
		t.Fatalf("lost updates: %d %v", len(cfg.Projects), err)
	}
}

func TestSettingsPersistenceAndDefaults(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	cfg, err := store.Load()
	if err != nil || cfg.Settings.LaunchMode("cli") != "window" || cfg.Settings.LaunchMode("run") != "window" {
		t.Fatal("new-window defaults missing")
	}
	_, err = store.Update(func(cfg *Config) error {
		cfg.Settings = Settings{Hidden: map[string]bool{"opencode": true}, Launch: map[string]string{"cli": "ask", "run": "same"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = store.Load()
	if err != nil || cfg.Settings.Shown("opencode") || cfg.Settings.LaunchMode("cli") != "ask" || cfg.Settings.LaunchMode("run") != "same" {
		t.Fatal("settings did not persist")
	}
	draft := cfg.Settings.Clone()
	draft.Hidden["opencode"] = false
	if cfg.Settings.Shown("opencode") {
		t.Fatal("draft changed saved settings")
	}
}

func TestLegacyTabPreferenceBecomesWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"projects":[],"settings":{"launch":{"cli":"tab","run":"ask"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	cfg, err := store.Load()
	if err != nil || cfg.Settings.LaunchMode("cli") != "window" || cfg.Settings.LaunchMode("run") != "ask" {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	if _, err := store.Update(func(*Config) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"tab"`) {
		t.Fatal("legacy tab preference retained")
	}
}

func TestAccentPersistsAndDefaults(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	cfg, _ := store.Load()
	if cfg.Settings.AccentColor().Name != "fatal pink" || cfg.Settings.AccentColor().Hex != "#fa11b0" {
		t.Fatal("default accent changed")
	}
	_, err := store.Update(func(cfg *Config) error { cfg.Settings.Accent = "violet"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = store.Load()
	if err != nil || cfg.Settings.AccentColor().Name != "violet" || cfg.Settings.Clone().Accent != "violet" {
		t.Fatal("accent did not persist")
	}
	if _, err := store.Update(func(cfg *Config) error { cfg.Settings.Accent = "unknown"; return nil }); err == nil {
		t.Fatal("invalid accent accepted")
	}
}

func TestProjectOverridesPersistIndependently(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	_, err := store.Update(func(cfg *Config) error {
		cfg.Settings.Hidden = map[string]bool{"opencode": true}
		cfg.Settings.Overrides = map[string]map[string]bool{"first": {"opencode": true}, "second": {"github": false}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil || !cfg.Settings.ShownFor("first", "opencode") || cfg.Settings.ShownFor("second", "opencode") || cfg.Settings.ShownFor("second", "github") {
		t.Fatal("overrides did not persist independently")
	}
	copy := cfg.Settings.Clone()
	copy.Overrides["first"]["opencode"] = false
	if !cfg.Settings.ShownFor("first", "opencode") {
		t.Fatal("draft override changed saved settings")
	}
}

func TestElectricBlueMigratesToGlacialBlue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"projects":[],"settings":{"accent":"electric-blue"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := New(path).Load()
	if err != nil || cfg.Settings.AccentColor().Name != "glacial blue" || cfg.Settings.AccentColor().Hex != "#8ccfe8" {
		t.Fatalf("migration failed: %+v %v", cfg.Settings, err)
	}
}

func TestStartupSettingAndRandomAccentPersist(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	cfg, _ := store.Load()
	if cfg.Settings.StartPage() != "overview" || !cfg.Settings.AutoOpenProject() {
		t.Fatal("startup must default to overview")
	}
	_, err := store.Update(func(cfg *Config) error {
		cfg.Settings.DefaultPage = "projects"
		cfg.Settings.ProjectOpening = "normal"
		cfg.Settings.Accent = "random"
		cfg.Settings.GraphScope = "all"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = store.Load()
	if err != nil || cfg.Settings.StartPage() != "projects" || cfg.Settings.AutoOpenProject() || cfg.Settings.AccentColor().ID != "random" || cfg.Settings.GitGraphScope() != "all" {
		t.Fatal("preferences did not persist")
	}
	copy := cfg.Settings.Clone()
	copy.ProjectOpening = "project"
	if cfg.Settings.StartPage() != "projects" || !copy.AutoOpenProject() {
		t.Fatal("snapshot changed original startup preference")
	}
}
