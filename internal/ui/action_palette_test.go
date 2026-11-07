package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rian/internal/git"
)

func TestPaletteGroupsAvailableActionsWithoutDuplicates(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	m.available["code"] = false
	m.available["kilocode"] = false
	m.available["files"] = false
	m.openMenu(palette)
	items := m.filteredActions()
	seen := map[string]bool{}
	group := -1
	for _, a := range items {
		if a.Disabled != "" || a.ID == "vscode" || a.ID == "kilocode" || a.ID == "files" {
			t.Fatalf("unavailable action visible: %+v", a)
		}
		key := a.ID + "/" + a.Value
		if seen[key] {
			t.Fatalf("duplicate: %s", key)
		}
		seen[key] = true
		if paletteGroup(a) < group {
			t.Fatal("groups interleaved")
		}
		group = paletteGroup(a)
	}
	if len(items) == 0 || items[len(items)-1].ID != "remove" {
		t.Fatal("Remove project not at bottom")
	}
	body := plain(m.paletteView(items, 65, 100))
	if strings.Contains(body, "\n\n") {
		t.Fatal("blank spacing between palette rows")
	}
	for _, heading := range []string{"Commands", "Git", "Project"} {
		if !strings.Contains(body, heading) {
			t.Fatalf("missing group %s", heading)
		}
	}
	m.query.SetValue("GitHub")
	if len(m.filteredActions()) > 1 {
		t.Fatal("duplicate GitHub remains during search")
	}
	m.query.Reset()
	m.statuses[p.ID] = git.Status{Error: "folder missing"}
	m.selected = 1
	m.openMenu(palette)
	for _, a := range m.filteredActions() {
		switch a.ID {
		case "vscode", "opencode", "kilocode", "terminal", "files", "run", "git", "refresh":
			t.Fatalf("missing-folder action shown: %s", a.ID)
		}
	}
}
func TestPaletteScrollingKeepsSelectionAndGroupVisible(t *testing.T) {
	m := fixture(t)
	m.mode = palette
	m.menu = []Action{{ID: "vscode", Label: "Editor"}, {ID: "url", Label: "URL", Value: "https://example.com"}, {ID: "run", Label: "Command"}, {ID: "git", Label: "Fetch", Value: "fetch"}, {ID: "settings", Label: "Settings"}, {ID: "remove", Label: "Remove project"}}
	for i := range m.menu {
		m.menuIndex = i
		body := plain(m.paletteView(m.menu, 40, 4))
		if !strings.Contains(body, "› "+m.menu[i].Label) {
			t.Fatalf("selection %d clipped: %s", i, body)
		}
		if !strings.Contains(body, paletteGroups[paletteGroup(m.menu[i])]) {
			t.Fatalf("selection %d lost group", i)
		}
		if lipgloss.Height(body) > 4 {
			t.Fatal("palette exceeded available rows")
		}
	}
	m.openMenu(palette)
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.resize()
		m.menuIndex = max(0, len(m.filteredActions())-1)
		view := m.View().Content
		if lipgloss.Height(view) > m.height || lipgloss.Width(view) > m.width {
			t.Fatalf("palette overflows at %v", size)
		}
		if !strings.Contains(plain(m.modalView()), "› Remove project") {
			t.Fatalf("Remove selection hidden at %v", size)
		}
	}
	m.menuIndex = 0
	m.modalKey(press(tea.KeyDown, ""))
	if m.menuIndex != 1 {
		t.Fatal("group heading intercepted navigation")
	}
}
