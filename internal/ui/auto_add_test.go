package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"rian/internal/config"
)

func TestAutoAddDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one", "two", ".hidden", "one/nested"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("text"), 0600)
	projects, err := discoverProjects(root, false)
	if err != nil || len(projects) != 2 {
		t.Fatalf("discovery %v: %v", projects, err)
	}
	projects, err = discoverProjects(root, true)
	if err != nil || len(projects) != 3 {
		t.Fatal("hidden folder option failed")
	}
	m := fixture(t)
	m.cfg.Settings.AutoAddEnabled = true
	m.cfg.Settings.AutoAddLocation = root
	m.autoAddKey = autoAddKey(m.cfg.Settings)
	m.autoAddCandidates = projects
	m.cfg.Projects = append(m.cfg.Projects, projects[0])
	m.cfg.AutoAddSkipped = map[string]bool{projects[1].Path: true}
	if len(m.pendingAutoAdd()) != 1 {
		t.Fatal("saved/skipped candidates still offered")
	}
	m.cfg.Settings.AutoAddEnabled = false
	if len(m.pendingAutoAdd()) != 0 {
		t.Fatal("disabled discovery still offered")
	}
}
func TestAutoAddReviewRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new-project")
	os.Mkdir(path, 0700)
	m := fixture(t)
	m.cfg.Settings.AutoAddEnabled = true
	m.cfg.Settings.AutoAddLocation = root
	_, err := m.store.Update(func(c *config.Config) error { *c = m.cfg; return nil })
	if err != nil {
		t.Fatal(err)
	}
	scan := m.scanAutoAdd()
	if scan == nil {
		t.Fatal("scan not started")
	}
	m.Update(scan())
	if len(m.pendingAutoAdd()) != 1 {
		t.Fatal("new project missing")
	}
	first := plain(m.headerStatus())
	m.autoAddBlink = !m.autoAddBlink
	if !strings.Contains(first, "A review 1 new project") || first == plain(m.headerStatus()) {
		t.Fatal("discovery notice not pulsing")
	}
	m.key(press('A', "A"))
	if m.mode != autoAddReview {
		t.Fatal("A did not open review")
	}
	cmd := m.modalKey(press(tea.KeyEnter, ""))
	m.Update(cmd())
	if m.mode != addConfirm || len(m.cfg.Projects) != 3 {
		t.Fatal("review saved before confirmation")
	}
	cmd = m.modalKey(press(tea.KeyEnter, ""))
	m.Update(cmd())
	if m.mode != autoAddReview || len(m.cfg.Projects) != 4 || len(m.pendingAutoAdd()) != 0 {
		t.Fatal("confirmed project not saved or queue not refreshed")
	}
}
func TestAutoAddSettingsCancelAndStaleScan(t *testing.T) {
	m := fixture(t)
	m.startSettings()
	m.openSettingsPage("behavior/auto add")
	if len(m.settingsMatches()) != 3 {
		t.Fatal("nested settings missing")
	}
	m.settingsKey("space")
	if !m.cfg.Settings.AutoAddEnabled {
		t.Fatal("enabled did not immediately apply")
	}
	m.settingsIndex = 1
	m.settingsKey("enter")
	if m.mode != autoAddLocation {
		t.Fatal("location input not opened")
	}
	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	m.query.SetValue(root)
	cmd := m.autoAddLocationKey(press(tea.KeyEnter, ""))
	if got := m.cfg.Settings.AutoAddLocation; got != canonicalRoot {
		t.Fatalf("location = %q, want canonical path %q", got, canonicalRoot)
	}
	m.settingsKey("ctrl+c")
	if m.cfg.Settings.AutoAddEnabled || m.cfg.Settings.AutoAddLocation != "" {
		t.Fatal("cancel did not restore discovery settings")
	}
	if cmd != nil {
		m.Update(cmd())
	}
	if len(m.pendingAutoAdd()) != 0 {
		t.Fatal("stale scan restored canceled settings")
	}
}
func TestAutoAddSettingsClone(t *testing.T) {
	s := config.Settings{AutoAddEnabled: true, AutoAddLocation: "/projects", AutoAddHidden: true}
	c := s.Clone()
	if c.AutoAddEnabled != s.AutoAddEnabled || c.AutoAddLocation != s.AutoAddLocation || c.AutoAddHidden != s.AutoAddHidden {
		t.Fatal("settings clone lost discovery configuration")
	}
}

func TestAutoAddSkipSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "skipped"), 0700)
	os.Mkdir(filepath.Join(root, "wanted"), 0700)
	m := fixture(t)
	m.cfg.Settings.AutoAddEnabled = true
	m.cfg.Settings.AutoAddLocation = root
	if _, err := m.store.Update(func(c *config.Config) error { *c = m.cfg; return nil }); err != nil {
		t.Fatal(err)
	}
	m.Update(m.scanAutoAdd()())
	m.openAutoAddReview()
	skipped := m.pendingAutoAdd()[0].Path
	cmd := m.autoAddReviewKey(press('x', "x"))
	if cmd == nil {
		t.Fatal("skip was not persisted")
	}
	m.Update(cmd())
	if !m.cfg.AutoAddSkipped[skipped] || len(m.pendingAutoAdd()) != 1 || m.mode != autoAddReview {
		t.Fatal("skip did not update review")
	}
	cfg, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	restarted := fixture(t)
	restarted.cfg = cfg
	restarted.Update(restarted.scanAutoAdd()())
	if len(restarted.pendingAutoAdd()) != 1 || restarted.pendingAutoAdd()[0].Path == skipped {
		t.Fatal("skipped folder reappeared after restart")
	}
	// Other settings edits and cancel must preserve permanent skips.
	restarted.startSettings()
	restarted.settingsDraft.AutoAddHidden = true
	restarted.applySettingsChange()
	restarted.settingsKey("ctrl+c")
	if !restarted.cfg.AutoAddSkipped[skipped] {
		t.Fatal("settings cancellation forgot skip")
	}
}
