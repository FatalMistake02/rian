package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"rian/internal/addon"
	"rian/internal/project"
)

func metadataModel(t *testing.T) (*Model, addon.Installed, project.Project) {
	t.Helper()
	m := fixture(t)
	p, _ := m.current()
	if err := os.WriteFile(filepath.Join(p.Path, "manifest.json"), []byte(`{"name":"widget","version":"1.0.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	item := testAddon()
	item.Definition.ID = "registry"
	item.Definition.Name = "Registry"
	item.Definition.Shortcut = "N"
	item.Definition.Menu = false
	item.Definition.Detect = addon.Detection{Files: []string{"manifest.json"}}
	item.Definition.Metadata = &addon.MetadataProvider{File: "manifest.json", Name: "${file.name}"}
	item.Definition.Actions = []addon.Action{{ID: "open", Name: "Open", Kind: "metadata-url"}}
	item.Definition.Pages = []addon.Page{{ID: "registry", Name: "Registry"}}
	m.cfg.Settings.Addons = map[string]addon.Installed{"registry": item}
	return m, item, p
}

func TestAddonMetadataSchedulingAndStaleResults(t *testing.T) {
	m, item, p := metadataModel(t)
	if cmd := m.refreshAddonMetadata(); cmd == nil {
		t.Fatal("metadata not scheduled")
	}
	if cmd := m.refreshAddonMetadata(); cmd != nil {
		t.Fatal("duplicate metadata load")
	}
	key := metadataKey("registry", p)
	fingerprint := metadataFingerprint(item, p)
	data := addon.Metadata{Name: "widget", Summary: "local 1.0.0 / latest 2.0.0", URL: "https://example.com/widget"}
	m.Update(addonMetadataMsg{key, fingerprint, data, nil})
	if cmd := m.refreshAddonMetadata(); cmd != nil {
		t.Fatal("cache not respected")
	}
	loaded, status := m.metadataFor("registry", p)
	if status != "" || loaded.URL != data.URL {
		t.Fatalf("missing data: %+v %s", loaded, status)
	}
	m.invalidateAddonMetadata(p.ID)
	if cmd := m.refreshAddonMetadata(); cmd == nil {
		t.Fatal("manual refresh not scheduled")
	}
	item.Enabled = false
	m.cfg.Settings.Addons["registry"] = item
	m.refreshAddonMetadata()
	m.Update(addonMetadataMsg{key, fingerprint, data, nil})
	if len(m.addonMetadata) != 0 {
		t.Fatal("disabled addon retained or accepted data")
	}
	item.Enabled = true
	m.cfg.Settings.Addons["registry"] = item
	for j := 0; j < 6; j++ {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{}`), 0600)
		m.cfg.Projects = append(m.cfg.Projects, project.New(fmt.Sprint(j), dir))
	}
	m.refreshAddonMetadata()
	if len(m.addonMetadata) != 4 {
		t.Fatalf("load limit: %d", len(m.addonMetadata))
	}
}

func TestAddonMetadataOverviewAndShortcut(t *testing.T) {
	m, item, p := metadataModel(t)
	key := metadataKey("registry", p)
	m.addonMetadata = map[string]addonMetadataState{key: {data: addon.Metadata{Name: "widget", Summary: "local 1.0.0 / latest 2.0.0", URL: "https://example.com/widget", Fields: []addon.MetadataField{{Label: "Local", Value: "1.0.0"}}}, fingerprint: metadataFingerprint(item, p), checked: time.Now()}}
	for _, key := range []string{"N", "shift+n"} {
		handled, cmd := m.addonShortcut(key)
		if !handled || cmd == nil {
			t.Fatalf("shortcut %s failed", key)
		}
	}
	if handled, _ := m.addonShortcut("n"); handled {
		t.Fatal("lowercase Notes key intercepted")
	}
	m.overview = true
	m.overviewIndex = 2
	m.overviewFocused = true
	page, _ := m.selectedAddonPage()
	if handled, cmd := m.addonPageKey("N"); !handled || cmd == nil {
		t.Fatal("overview shortcut failed")
	}
	for _, size := range [][2]int{{70, 24}, {24, 8}, {35, 12}} {
		view := plain(m.addonPageView(page, size[0], size[1]))
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("view overflow at %v", size)
		}
		if !strings.Contains(view, "widget") {
			t.Fatalf("package row missing at %v: %s", size, view)
		}
	}
	if !strings.Contains(plain(m.detailView(70, 30)), "1.0.0") {
		t.Fatal("project metadata missing")
	}
	state := m.addonMetadata[key]
	state.data.Hidden = true
	m.addonMetadata[key] = state
	if len(m.addonPageActions(page)) != 0 {
		t.Fatal("hidden metadata row shown")
	}
}
