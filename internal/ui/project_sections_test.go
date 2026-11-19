package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"rian/internal/addon"
	"rian/internal/config"
)

func TestProjectSectionReorderPreviewSaveCancelAndReset(t *testing.T) {
	m := fixture(t)
	if _, err := m.store.Update(func(cfg *config.Config) error { *cfg = m.cfg; return nil }); err != nil {
		t.Fatal(err)
	}
	m.startSettings()
	m.openSettingsPage("section order")
	rows := m.settingsMatches()
	if len(rows) != 5 || rows[0].id != "stack" {
		t.Fatalf("missing section rows: %+v", rows)
	}
	m.settingsIndex = 4
	m.settingsKey("ctrl+up")
	if m.cfg.Settings.ProjectSections()[3].ID != "commands" || m.settingsMatches()[m.settingsIndex].id != "commands" {
		t.Fatal("move/selection failed")
	}
	m.settingsKey("left")
	if m.cfg.Settings.ProjectSections()[2].ID != "commands" {
		t.Fatal("left move failed")
	}
	m.settingsKey("ctrl+c")
	if len(m.cfg.Settings.ProjectSectionOrder) != 0 {
		t.Fatal("cancel retained preview")
	}
	m.startSettings()
	m.openSettingsPage("section order")
	m.settingsIndex = 0
	m.settingsKey("ctrl+up")
	if m.settingsChanged {
		t.Fatal("moving beyond first row changed settings")
	}
	m.settingsKey("right")
	want := append([]string{}, m.cfg.Settings.ProjectSectionOrder...)
	cmd := m.settingsKey("ctrl+s")
	if cmd == nil {
		t.Fatal("missing save")
	}
	m.Update(cmd())
	cfg, err := m.store.Load()
	if err != nil || !reflect.DeepEqual(cfg.Settings.ProjectSectionOrder, want) {
		t.Fatalf("save failed: %+v %v", cfg.Settings, err)
	}
	m.startSettings()
	m.openSettingsPage("section order")
	m.settingsKey("backspace")
	if len(m.cfg.Settings.ProjectSectionOrder) != 0 || m.settingsMatches()[0].id != "stack" {
		t.Fatal("reset failed")
	}
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.resize()
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("settings overflow at %v", size)
		}
	}
}

func TestProjectDetailsFollowSectionOrderIncludingAddons(t *testing.T) {
	m, item, p := metadataModel(t)
	m.projectFull = true
	m.cfg.Settings.ProjectSectionOrder = []string{"commands", "github", "addon/registry", "commit", "git", "stack"}
	m.addonMetadata = map[string]addonMetadataState{metadataKey("registry", p): {fingerprint: metadataFingerprint(item, p), checked: time.Now(), data: addon.Metadata{Title: "REGISTRY DETAILS", Name: "widget"}}}
	view := plain(strings.Join(m.projectDetailSections(p, 90), "\n"))
	previous := -1
	for _, label := range []string{"COMMANDS", "GITHUB", "REGISTRY DETAILS", "LAST COMMIT", "Branch", "Node.js"} {
		position := strings.Index(view, label)
		if position < 0 || position <= previous {
			t.Fatalf("section %s misplaced in %s", label, view)
		}
		previous = position
	}
	m.startSettings()
	m.openSettingsPage("section order")
	for index, row := range m.settingsMatches() {
		if row.id == "addon/registry" {
			m.settingsIndex = index
			break
		}
	}
	m.settingsKey("ctrl+up")
	if m.cfg.Settings.ProjectSections()[1].ID != "addon/registry" {
		t.Fatal("addon section not moved individually")
	}
}
