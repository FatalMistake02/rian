package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"rian/internal/config"
	"rian/internal/project"
)

func (m *Model) startNotes(p project.Project) tea.Cmd {
	m.mode, m.formError = notes, ""
	m.notesProject = p
	m.notesInput = textarea.New()
	m.notesInput.Placeholder = "Write a note"
	m.notesInput.Prompt = ""
	m.notesInput.ShowLineNumbers = false
	m.notesInput.CharLimit = 0
	m.styleNotesInput()
	m.notesInput.SetValue(plain(p.Notes))
	m.resize()
	return m.notesInput.Focus()
}

func (m *Model) styleNotesInput() {
	s := m.notesInput.Styles()
	s.Focused.Base = textStyle.Background(background)
	s.Focused.Text = textStyle
	s.Focused.CursorLine = selectedStyle
	s.Focused.Selection = selectedStyle
	s.Focused.Prompt = highlight
	s.Focused.Placeholder = muted
	s.Focused.EndOfBuffer = muted
	s.Blurred.Text = textStyle
	s.Blurred.Selection = selectedStyle
	s.Cursor.Color = accent
	m.notesInput.SetStyles(s)
}
func (m *Model) notesKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.notesInput.Blur()
		m.closeModal()
		return nil
	case "ctrl+s":
		id, value := m.notesProject.ID, strings.TrimSpace(plain(m.notesInput.Value()))
		return m.save(id, "Notes saved", func(cfg *config.Config) error {
			for i := range cfg.Projects {
				if cfg.Projects[i].ID == id {
					cfg.Projects[i].Notes = value
					return nil
				}
			}
			return fmt.Errorf("project no longer exists")
		})
	}
	var cmd tea.Cmd
	m.notesInput, cmd = m.notesInput.Update(msg)
	return cmd
}
func (m *Model) notesView() (string, string, string) {
	return "NOTES / " + oneLine(m.notesProject.Name), m.notesInput.View(), keyHint("Ctrl+S", "save") + "   " + keyHint("Esc", "cancel")
}

func wrappedProjectNotes(p project.Project, width int) []string {
	text := strings.TrimSpace(plain(p.Notes))
	if text == "" {
		text = "No notes yet."
	}
	return strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
}
func projectNotesView(p project.Project, width, height int) string {
	return scrolledProjectNotesView(p, width, height, 0)
}
func scrolledProjectNotesView(p project.Project, width, height, offset int) string {
	wrapped := wrappedProjectNotes(p, width)
	capacity := max(0, height-1)
	offset = min(max(0, offset), max(0, len(wrapped)-capacity))
	end := min(len(wrapped), offset+capacity)
	header := titleStyle.Render("NOTES")
	if offset > 0 {
		header += muted.Render(" ↑")
	}
	if end < len(wrapped) {
		header += muted.Render(" ↓ more")
	}
	lines := []string{fit(header, width)}
	for _, line := range wrapped[offset:end] {
		lines = append(lines, textStyle.Render(fit(line, width)))
	}
	return strings.Join(lines, "\n")
}
func (m *Model) notesPaneSize() (int, int) {
	w, h := m.width-4, m.height-2
	leftW := max(23, w*45/100)
	height := h - 2
	if p, ok := m.current(); ok && m.statuses[p.ID].Repository {
		height = max(5, min(10, h/3)) - 2
	}
	return w - leftW - 5, height
}
func (m *Model) resetNotesScroll() {
	p, ok := m.current()
	if ok && p.ID != m.notesScrollProject {
		m.notesScrollProject = p.ID
		m.notesOffset = 0
	}
}
func (m *Model) notesPaneKey(key string) (bool, tea.Cmd) {
	m.resetNotesScroll()
	if !m.notesFocused {
		if !m.graphFocused && (key == "down" || key == "k") {
			m.notesFocused = true
			return true, nil
		}
		return false, nil
	}
	p, ok := m.current()
	if !ok {
		return false, nil
	}
	width, height := m.notesPaneSize()
	last := max(0, len(wrappedProjectNotes(p, width))-max(1, height-1))
	m.notesOffset = min(m.notesOffset, last)
	switch key {
	case "left", "esc":
		m.notesFocused = false
	case "up", "j":
		m.notesOffset = max(0, m.notesOffset-1)
	case "down", "k":
		m.notesOffset = min(last, m.notesOffset+1)
	case "pgup":
		m.notesOffset = max(0, m.notesOffset-max(1, height-1))
	case "pgdown":
		m.notesOffset = min(last, m.notesOffset+max(1, height-1))
	case "home":
		m.notesOffset = 0
	case "end":
		m.notesOffset = last
	default:
		return false, nil
	}
	return true, nil
}
