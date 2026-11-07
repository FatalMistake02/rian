package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type overviewAlert struct{ projectID, name, detail string }

func (m *Model) overviewAlerts() ([]overviewAlert, bool) {
	alerts := []overviewAlert{}
	pending := false
	for _, p := range m.cfg.Projects {
		s, known := m.statuses[p.ID]
		if !known {
			pending = true
			continue
		}
		details := []string{}
		if s.Error != "" {
			details = append(details, oneLine(s.Error))
		} else if s.Repository {
			if s.Dirty() {
				details = append(details, "Uncommitted changes")
			}
			if s.Ahead > 0 {
				details = append(details, fmt.Sprintf("%d to push", s.Ahead))
			}
			if s.Behind > 0 {
				details = append(details, fmt.Sprintf("%d incoming", s.Behind))
			}
		}
		if len(details) > 0 {
			alerts = append(alerts, overviewAlert{p.ID, oneLine(p.Name), strings.Join(details, " · ")})
		}
	}
	return alerts, pending
}

func (m *Model) overviewKey(key string) (bool, tea.Cmd) {
	if _, ok := m.selectedAddonPage(); ok && m.overviewFocused {
		return m.addonPageKey(key)
	}
	pages := m.overviewPageNames()
	if m.overviewIndex >= len(pages) {
		m.overviewFocused = false
	}
	m.overviewIndex = min(m.overviewIndex, len(pages)-1)
	alerts, _ := m.overviewAlerts()
	if m.overviewFocused {
		m.overviewAlertIndex = min(m.overviewAlertIndex, max(0, len(alerts)-1))
		switch key {
		case "left", "esc", "tab", "shift+tab":
			m.overviewFocused = false
		case "up", "j":
			m.overviewAlertIndex = max(0, m.overviewAlertIndex-1)
		case "down", "k":
			m.overviewAlertIndex = min(max(0, len(alerts)-1), m.overviewAlertIndex+1)
		case "home":
			m.overviewAlertIndex = 0
		case "end":
			m.overviewAlertIndex = max(0, len(alerts)-1)
		case "enter", "space", " ":
			if len(alerts) > 0 {
				m.search.Reset()
				m.selectID(alerts[m.overviewAlertIndex].projectID)
				m.overview, m.projectFull, m.overviewFocused = false, true, false
				m.graphFocused, m.graphIndex = false, 0
				m.restoreGraphSelection("")
			}
		case "a", "+", "s", "?", "q", "ctrl+k":
			return false, nil
		}
		return true, nil
	}
	switch key {
	case "home":
		m.overviewIndex = 0
	case "end":
		m.overviewIndex = len(pages) - 1
	case "up", "j", "shift+tab":
		m.overviewIndex = max(0, m.overviewIndex-1)
	case "down", "k":
		m.overviewIndex = min(len(pages)-1, m.overviewIndex+1)
	case "right", "tab":
		if _, ok := m.selectedAddonPage(); ok {
			m.overviewFocused = true
			return true, nil
		}
		if len(alerts) > 0 {
			m.overviewFocused = true
			return true, nil
		}
		if key == "tab" {
			m.overviewIndex = (m.overviewIndex + 1) % len(pages)
			return true, nil
		}
		fallthrough
	case "enter", "space", " ":
		if _, ok := m.selectedAddonPage(); ok {
			m.overviewFocused = true
			return true, nil
		}
		if m.overviewIndex == 1 {
			m.startSettings()
			return true, nil
		}
		m.overview, m.projectFull = false, false
	case "a", "+", "s", "?", "q", "ctrl+k":
		return false, nil
	}
	return true, nil
}

func (m *Model) overviewMenu(width, height int) string {
	lines := []string{}
	pages := m.overviewPageNames()
	height = max(1, height)
	selected := min(m.overviewIndex, len(pages)-1)
	start := max(0, selected-height+1)
	for i := start; i < min(len(pages), start+height); i++ {
		prefix := "  "
		if i == m.overviewIndex {
			prefix = "› "
		}
		row := pad(prefix+oneLine(pages[i]), width)
		if i == m.overviewIndex && !m.overviewFocused {
			row = selectedStyle.Render(row)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) overviewView(width, height int) string {
	if page, ok := m.selectedAddonPage(); ok {
		return m.addonPageView(page, width, height)
	}
	lines := []string{titleStyle.Render("OVERVIEW"), "", highlight.Bold(true).Render("Welcome to Rian"), ""}
	alerts, pending := m.overviewAlerts()
	if len(m.cfg.Projects) == 0 {
		lines = append(lines, "Add your first project", "to get started.", "", keyHint("a", "add project"))
	} else if len(alerts) > 0 {
		lines = append(lines, titleStyle.Render("NEEDS ATTENTION"), "")
		count := max(1, (height-len(lines))/3)
		selected := min(m.overviewAlertIndex, len(alerts)-1)
		start := max(0, selected-count+1)
		for i := start; i < min(len(alerts), start+count); i++ {
			prefix := "  "
			if m.overviewFocused && i == selected {
				prefix = "› "
			}
			row := pad(fit(prefix+alerts[i].name, width), width)
			if m.overviewFocused && i == selected {
				row = selectedStyle.Render(row)
			} else {
				row = textStyle.Bold(true).Render(row)
			}
			lines = append(lines, row, muted.Render(fit("  "+alerts[i].detail, width)), "")
		}
	} else if pending {
		lines = append(lines, m.spin.View()+muted.Render(" Checking project status"))
	} else {
		lines = append(lines, good.Render("All caught up."), "", muted.Render("No uncommitted or unsynced"), muted.Render("Git changes detected."))
	}
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines[:min(len(lines), max(1, height))], "\n")
}
