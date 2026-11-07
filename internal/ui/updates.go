package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"rian/internal/updates"
	"strings"
)

type releaseMsg struct {
	version string
	err     error
}
type updatedMsg struct{ err error }

func checkRelease(version string) tea.Cmd {
	if version == "" {
		return nil
	}
	return func() tea.Msg { v, err := updates.Check(version); return releaseMsg{v, err} }
}
func (m *Model) updateMenuKey(key string) tea.Cmd {
	switch key {
	case "esc":
		m.closeModal()
	case "up", "j":
		m.menuIndex = 0
	case "down", "k":
		m.menuIndex = 1
	case "enter":
		if m.menuIndex == 1 {
			m.latestRelease = ""
			m.closeModal()
			return nil
		}
		cmd, err := updates.Command(m.latestRelease)
		if err != nil {
			return m.notify("Update unavailable", err)
		}
		m.closeModal()
		m.updating = true
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return updatedMsg{err} })
	}
	return nil
}
func (m *Model) updateMenuView() (string, string, string) {
	lines := []string{fmt.Sprintf("Rian %s → %s", m.version, m.latestRelease), "", "  Update", "  Dismiss"}
	lines[2+m.menuIndex] = highlight.Render("› " + strings.TrimSpace(lines[2+m.menuIndex]))
	return "UPDATE AVAILABLE", strings.Join(lines, "\n"), keyHint("↑↓", "choose") + "  " + keyHint("Enter", "select") + "  " + keyHint("Esc", "back")
}
func (m *Model) headerStatus() string {
	return m.headerNotices(false)
}

func (m *Model) headerStatusWidth(width int) string {
	status := m.headerStatus()
	if lipgloss.Width(status) > width {
		status = m.headerNotices(true)
	}
	return fit(status, width)
}

func (m *Model) headerNotices(compact bool) string {
	var notices []string
	if count := len(m.pendingAutoAdd()); count > 0 {
		marker := "●"
		if !m.autoAddBlink {
			marker = "○"
		}
		noun := "projects"
		if count == 1 {
			noun = "project"
		}
		if compact {
			notices = append(notices, keyHint("A", fmt.Sprint(count)))
		} else {
			notices = append(notices, highlight.Render(marker+" ")+keyHint("A", fmt.Sprintf("review %d new %s", count, noun)))
		}
	}
	if m.updating {
		notices = append(notices, muted.Render("Updating…"))
	} else if m.latestRelease != "" {
		if compact {
			notices = append(notices, keyHint("u", "update"))
		} else {
			notices = append(notices, highlight.Render("● ")+keyHint("u", "update "+m.latestRelease))
		}
	}
	if count := len(m.pendingAddonUpdates()); count > 0 {
		if compact {
			notices = append(notices, keyHint("U", fmt.Sprint(count)))
		} else {
			noun := "updates"
			if count == 1 {
				noun = "update"
			}
			notices = append(notices, highlight.Render("● ")+keyHint("U", fmt.Sprintf("review %d addon %s", count, noun)))
		}
	}
	return strings.Join(notices, "   ")
}
