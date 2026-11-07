package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rian/internal/config"
)

func TestProjectNotesSaveAndCancel(t *testing.T) {
	m := fixture(t)
	m.cfg.Projects[0].Notes = "Existing note"
	if _, err := m.store.Update(func(c *config.Config) error { *c = m.cfg; return nil }); err != nil {
		t.Fatal(err)
	}
	m.key(press('n', "n"))
	if m.mode != notes || m.notesInput.Value() != "Existing note" {
		t.Fatal("notes shortcut did not open existing note")
	}
	m.notesInput.SetValue("First line\nSecond line")
	m.notesKey(press(tea.KeyEscape, ""))
	if m.cfg.Projects[0].Notes != "Existing note" {
		t.Fatal("cancel modified stored notes")
	}
	m.key(press('n', "n"))
	m.notesInput.SetValue("First line\nSecond line")
	cmd := m.notesKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m.Update(cmd())
	stored, err := m.store.Load()
	if err != nil || stored.Projects[0].Notes != "First line\nSecond line" {
		t.Fatalf("notes not persisted: %v", err)
	}
	m.projectFull = true
	if !strings.Contains(plain(m.homeView()), "First line") {
		t.Fatal("project notes preview missing")
	}
	m.key(press('n', "n"))
	m.notesInput.SetValue("")
	cmd = m.notesKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m.Update(cmd())
	if m.cfg.Projects[0].Notes != "" {
		t.Fatal("clearing notes failed")
	}
}
func TestNotesTypingLayoutAndVisibility(t *testing.T) {
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m := fixture(t)
		m.width, m.height = size[0], size[1]
		m.key(press('n', "n"))
		m.key(press('q', "q"))
		m.key(press(tea.KeyEnter, ""))
		m.key(press('n', "n"))
		if m.notesInput.Value() != "q\nn" {
			t.Fatalf("editor shortcuts interrupted typing: %q", m.notesInput.Value())
		}
		view := plain(m.View().Content)
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height || !strings.Contains(view, "Ctrl+S save") || !strings.Contains(view, "Esc cancel") {
			t.Fatalf("notes layout at %v: %s", size, view)
		}
		m.notesKey(press(tea.KeyEscape, ""))
		m.cfg.Settings.Hidden = map[string]bool{"notes": true}
		m.key(press('n', "n"))
		if m.mode == notes {
			t.Fatal("hidden notes shortcut opened editor")
		}
	}
}

func TestNotesPanelsAndBottomPreview(t *testing.T) {
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m := fixture(t)
		m.width, m.height = size[0], size[1]
		m.cfg.Projects[0].Notes = "Reminder"
		if !strings.Contains(plain(m.homeView()), "Reminder") {
			t.Fatalf("normal notes missing at %v", size)
		}
		preview := plain(m.detailView(40, 24))
		noteAt := strings.Index(preview, "Reminder")
		if noteAt < strings.Index(preview, "Branch") || noteAt > strings.Index(preview, "n Notes") {
			t.Fatal("notes not at bottom above shortcuts")
		}
		m.projectFull = true
		view := plain(m.homeView())
		if strings.Count(view, "╭") != 3 || !strings.Contains(view, "Reminder") || strings.Index(view, "NOTES") < strings.Index(view, "GIT GRAPH") {
			t.Fatalf("notes panel missing at %v: %s", size, view)
		}
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatal("notes panels overflow terminal")
		}
		p, _ := m.current()
		status := m.statuses[p.ID]
		status.Repository = false
		m.statuses[p.ID] = status
		view = plain(m.homeView())
		if strings.Count(view, "╭") != 2 || !strings.Contains(view, "Reminder") {
			t.Fatal("non-Git project notes missing")
		}
	}
}

func TestNotesScrollingAndFocus(t *testing.T) {
	m := fixture(t)
	m.width, m.height = 80, 24
	m.projectFull = true
	m.cfg.Projects[0].Notes = "first\nsecond\nthird\nfourth\nfifth\nsixth\nseventh\nlast"
	view := plain(m.homeView())
	if !strings.Contains(view, "↓ more") {
		t.Fatal("overflow indicator missing")
	}
	m.key(press(tea.KeyDown, ""))
	if !m.notesFocused || m.graphFocused {
		t.Fatal("down did not focus notes")
	}
	m.key(press(tea.KeyDown, ""))
	if m.notesOffset != 1 {
		t.Fatal("down did not scroll notes")
	}
	m.key(press(tea.KeyEnd, ""))
	view = plain(m.homeView())
	if !strings.Contains(view, "last") || !strings.Contains(view, "NOTES ↑") || strings.Contains(view, "↓ more") {
		t.Fatal("end or scroll indicators incorrect")
	}
	m.key(press(tea.KeyLeft, ""))
	if m.notesFocused || !m.projectFull {
		t.Fatal("left did not return to project details")
	}
	m.key(press(tea.KeyDown, ""))
	m.key(press(tea.KeyEscape, ""))
	if m.notesFocused || !m.projectFull {
		t.Fatal("esc left project instead of notes")
	}
	// Down after the final graph commit moves to the box below it.
	m.graphFocused = true
	m.key(press(tea.KeyDown, ""))
	if !m.notesFocused || m.graphFocused {
		t.Fatal("empty graph did not navigate down to notes")
	}
	m.key(press('n', "n"))
	if m.mode != notes {
		t.Fatal("n did not edit focused notes")
	}
}
func TestNormalNotesFlowWithDetails(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	status := m.statuses[p.ID]
	status.Repository = false
	status.GitHub = ""
	m.statuses[p.ID] = status
	m.cfg.Projects[0].Notes = "Reminder"
	view := plain(m.detailView(70, 35))
	noteAt := strings.Index(view, "NOTES")
	detailsAt := strings.Index(view, "No Git repository detected.")
	if noteAt < detailsAt || strings.Count(view[detailsAt:noteAt], "\n") > 3 {
		t.Fatal("normal notes pushed away from preceding details")
	}
	m.cfg.Projects[0].Notes = strings.Repeat("Note line\n", 8) + "Final line"
	view = plain(m.detailView(70, 35))
	if !strings.Contains(view, "Final line") || strings.Contains(view, "↓ more") {
		t.Fatal("notes were cut off despite available space")
	}
	view = plain(m.detailView(70, 12))
	if !strings.Contains(view, "↓ more") {
		t.Fatal("notes overflow indicator missing when space runs out")
	}
}
