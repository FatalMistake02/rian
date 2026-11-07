package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/config"
	"rian/internal/project"
	"rian/internal/tools"
)

type autoAddSkippedMsg struct {
	cfg config.Config
	err error
}

type autoAddScanMsg struct {
	key      string
	projects []project.Project
	err      error
}

func autoAddKey(s config.Settings) string {
	return fmt.Sprintf("%t:%s:%t", s.AutoAddEnabled, s.AutoAddLocation, s.AutoAddHidden)
}

// Discovery only lists immediate directories. Inspection and saving require review.
func discoverProjects(location string, hidden bool) ([]project.Project, error) {
	root, err := project.CanonicalPath(location)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	projects := []project.Project{}
	for _, entry := range entries {
		if !entry.IsDir() || (!hidden && strings.HasPrefix(entry.Name(), ".")) {
			continue
		}
		path, err := project.CanonicalPath(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		projects = append(projects, project.New(entry.Name(), path))
	}
	return projects, nil
}
func (m *Model) scanAutoAdd() tea.Cmd {
	s := m.cfg.Settings
	key := autoAddKey(s)
	if key != m.autoAddKey {
		m.autoAddKey = key
		m.autoAddCandidates = nil
		m.autoAddLastScan = time.Time{}
		m.autoAddScanError = ""
	}
	if !s.AutoAddEnabled || s.AutoAddLocation == "" {
		m.autoAddCandidates = nil
		return nil
	}
	if m.autoAddScanning || time.Since(m.autoAddLastScan) < 10*time.Second {
		return nil
	}
	m.autoAddScanning = true
	m.autoAddLastScan = time.Now()
	return func() tea.Msg {
		projects, err := discoverProjects(s.AutoAddLocation, s.AutoAddHidden)
		return autoAddScanMsg{key, projects, err}
	}
}
func (m *Model) pendingAutoAdd() []project.Project {
	if !m.cfg.Settings.AutoAddEnabled || m.autoAddKey != autoAddKey(m.cfg.Settings) {
		return nil
	}
	saved := map[string]bool{}
	for _, p := range m.cfg.Projects {
		saved[p.Path] = true
	}
	items := []project.Project{}
	for _, p := range m.autoAddCandidates {
		if !saved[p.Path] && !m.cfg.AutoAddSkipped[p.Path] {
			items = append(items, p)
		}
	}
	return items
}
func (m *Model) openAutoAddReview() {
	m.mode, m.autoAddReviewIndex, m.formError = autoAddReview, 0, ""
	m.autoAddReviewing = true
}
func (m *Model) autoAddReviewKey(msg tea.KeyPressMsg) tea.Cmd {
	items := m.pendingAutoAdd()
	switch msg.String() {
	case "esc":
		m.autoAddReviewing = false
		m.closeModal()
	case "up", "j":
		m.autoAddReviewIndex = max(0, m.autoAddReviewIndex-1)
	case "down", "k":
		m.autoAddReviewIndex = min(max(0, len(items)-1), m.autoAddReviewIndex+1)
	case "x":
		if len(items) > 0 {
			path := items[min(m.autoAddReviewIndex, len(items)-1)].Path
			store := m.store
			m.busy = true
			return func() tea.Msg {
				cfg, err := store.Update(func(cfg *config.Config) error {
					if cfg.AutoAddSkipped == nil {
						cfg.AutoAddSkipped = map[string]bool{}
					}
					cfg.AutoAddSkipped[path] = true
					return nil
				})
				return autoAddSkippedMsg{cfg: cfg, err: err}
			}
		}
	case "enter":
		if len(items) > 0 {
			path := items[min(m.autoAddReviewIndex, len(items)-1)].Path
			m.mode = add
			m.query = input("Directory path")
			m.query.SetValue(path)
			return m.inspectDirectory("add", path, "")
		}
	}
	return nil
}
func (m *Model) autoAddReviewView(inner int) (string, string, string) {
	items := m.pendingAutoAdd()
	lines := []string{}
	count := max(1, m.height-12)
	start := max(0, m.autoAddReviewIndex-count+1)
	for i := start; i < min(len(items), start+count); i++ {
		row := "  " + oneLine(items[i].Name) + muted.Render("  "+oneLine(items[i].Path))
		if i == m.autoAddReviewIndex {
			row = highlight.Render("› " + fit(oneLine(items[i].Name)+"  "+oneLine(items[i].Path), inner-2))
		}
		lines = append(lines, row)
	}
	if len(items) == 0 {
		lines = append(lines, muted.Render("No new projects to review."))
	}
	footer := keyHint("Esc", "back")
	if len(items) > 0 {
		footer += "  " + keyHint("↑↓", "choose") + "  " + keyHint("Enter", "review") + "  " + keyHint("x", "skip")
	}
	return "AUTO ADD", strings.Join(lines, "\n"), footer
}
func (m *Model) autoAddLocationKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return m.settingsKey("ctrl+c")
	case "esc":
		m.mode = settings
		m.query.Blur()
		return nil
	case "enter":
		if strings.TrimSpace(m.query.Value()) == "" {
			m.busy = true
			return func() tea.Msg { path, err := tools.PickFolder(); return pickedFolderMsg{path: path, err: err} }
		}
		return m.setAutoAddLocation(m.query.Value())
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	return cmd
}
func (m *Model) setAutoAddLocation(path string) tea.Cmd {
	location, err := project.CanonicalPath(path)
	if err != nil {
		m.formError = err.Error()
		return m.query.Focus()
	}
	m.settingsDraft.AutoAddLocation = location
	m.applySettingsChange()
	m.mode, m.formError = settings, ""
	return m.scanAutoAdd()
}
