package ui

import (
	"os"
	"strings"

	"rian/internal/tools"
)

var paletteGroups = []string{"Navigation", "Tools", "Links", "Commands", "Git", "Addons", "Rian", "Project", "Actions"}

func paletteGroup(a Action) int {
	switch a.ID {
	case "projects", "overviewPage":
		return 0
	case "vscode", "opencode", "kilocode", "terminal", "files":
		return 1
	case "url":
		return 2
	case "run", "adhoc", "custom":
		return 3
	case "git", "commit", "branch", "switchBranch", "newBranch", "addRemote", "github", "lazygit", "remoteChoice":
		return 4
	case "settings":
		return 6
	case "pin", "edit", "refresh", "add", "remove", "notes":
		return 7
	}
	if strings.HasPrefix(a.ID, "addon/") || strings.HasPrefix(a.ID, "addon-action/") {
		return 5
	}
	return 8
}
func (m *Model) paletteActionAvailable(a Action) bool {
	if a.Disabled != "" {
		return false
	}
	if m.overview {
		return true
	}
	p, ok := m.current()
	if !ok {
		return a.ID == "add" || a.ID == "settings"
	}
	switch a.ID {
	case "remove":
		return !m.manager.Snapshot(p.ID).Running
	case "url":
		return a.Value != "" && tools.ValidateURL(a.Value) == nil && tools.BrowserAvailable()
	case "github":
		return tools.BrowserAvailable()
	case "run":
		if strings.TrimSpace(p.AllCommands()[a.Value]) == "" {
			return false
		}
	}
	switch a.ID {
	case "vscode", "opencode", "kilocode", "terminal", "files", "run", "adhoc", "git", "commit", "branch", "switchBranch", "newBranch", "addRemote", "lazygit", "refresh":
		info, err := os.Stat(p.Path)
		return err == nil && info.IsDir()
	}
	return true
}

// Group headings are display rows, never selectable actions. Starting a
// scrolled window within a group repeats its heading for context.
func (m *Model) paletteView(items []Action, width, capacity int) string {
	if len(items) == 0 {
		return muted.Render("No matching actions.")
	}
	selected := min(m.menuIndex, len(items)-1)
	capacity = max(2, capacity)
	start := max(0, selected-capacity+2)
	build := func(first int) ([]string, int) {
		rows := []string{}
		previous := -1
		selectedRow := 0
		for i := first; i < len(items); i++ {
			group := paletteGroup(items[i])
			if group != previous {
				rows = append(rows, muted.Bold(true).Render(paletteGroups[group]))
				previous = group
			}
			prefix := "  "
			if i == selected {
				prefix = "› "
				selectedRow = len(rows)
			}
			text := pad(prefix+oneLine(items[i].Label), width)
			if i == selected {
				text = selectedStyle.Render(text)
			}
			rows = append(rows, text)
		}
		return rows, selectedRow
	}
	rows, selectedRow := build(start)
	for selectedRow >= capacity && start < selected {
		start++
		rows, selectedRow = build(start)
	}
	lines := []string{}
	for _, r := range rows[:min(len(rows), capacity)] {
		lines = append(lines, fit(r, width))
	}
	return strings.Join(lines, "\n")
}
