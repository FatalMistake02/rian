package ui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"os"
	"path/filepath"
	"rian/internal/tools"
	"strings"

	"rian/internal/config"
	"rian/internal/git"
	"rian/internal/project"
)

func (m *Model) openStartupProject() {
	if !m.cfg.Settings.AutoOpenProject() || m.startupDir == "" {
		return
	}
	dir, err := filepath.EvalSymlinks(m.startupDir)
	if err != nil {
		return
	}
	bestID, bestLength := "", 0
	for _, p := range m.cfg.Projects {
		root, err := filepath.EvalSymlinks(p.Path)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(root, dir)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			continue
		}
		if project.SamePath(root, dir) || !filepath.IsAbs(relative) {
			if len(root) > bestLength {
				bestID, bestLength = p.ID, len(root)
			}
		}
	}
	if bestID != "" {
		m.search.Reset()
		m.selectID(bestID)
		m.projectFull = true
		m.overview = false
	}
}

func (m *Model) graphKey(key string) (bool, tea.Cmd) {
	p, ok := m.current()
	if !ok || !m.statuses[p.ID].Repository {
		return false, nil
	}
	if m.notesFocused {
		return false, nil
	}
	rows := m.graphRows(m.statuses[p.ID])
	indexes := []int{}
	for i, row := range rows {
		if row.Hash != "" {
			indexes = append(indexes, i)
		}
	}
	if key == "right" || key == "tab" && !m.graphFocused {
		m.graphFocused = true
		if len(indexes) > 0 && (m.graphIndex >= len(rows) || rows[m.graphIndex].Hash == "") {
			m.graphIndex = indexes[0]
		}
		return true, nil
	}
	if !m.graphFocused {
		return false, nil
	}
	switch key {
	case "b":
		hash := m.selectedGraphHash()
		scope := "all"
		if m.cfg.Settings.GitGraphScope() == "all" {
			scope = "current"
		}
		m.cfg.Settings.GraphScope = scope
		m.restoreGraphSelection(hash)
		return true, m.save(p.ID, "Git graph view updated", func(cfg *config.Config) error { cfg.Settings.GraphScope = scope; return nil })
	case "left", "esc", "tab", "shift+tab":
		m.graphFocused = false
		return true, nil
	case "up", "down", "j", "k", "home", "end":
		if len(indexes) == 0 {
			if key == "down" || key == "k" {
				m.graphFocused = false
				m.notesFocused = true
			}
			return true, nil
		}
		position := 0
		for i, index := range indexes {
			if index <= m.graphIndex {
				position = i
			}
		}
		switch key {
		case "up", "j":
			position = max(0, position-1)
		case "down", "k":
			if position == len(indexes)-1 {
				m.graphFocused = false
				m.notesFocused = true
				return true, nil
			}
			position = min(len(indexes)-1, position+1)
		case "home":
			position = 0
		case "end":
			position = len(indexes) - 1
		}
		m.graphIndex = indexes[position]
		return true, nil
	case "enter":
		url, err := m.graphCommitURL()
		if err != nil {
			return true, m.notify("Unable to open commit", err)
		}
		launcher := m.launcher
		return true, func() tea.Msg {
			return actionMsg{id: p.ID, notice: "Commit opened on GitHub", err: launcher.OpenURL(url)}
		}
	}
	return false, nil
}

func (m *Model) graphCommitURL() (string, error) {
	p, ok := m.current()
	if !ok {
		return "", fmt.Errorf("select a project first")
	}
	s := m.statuses[p.ID]
	rows := m.graphRows(s)
	if m.graphIndex < 0 || m.graphIndex >= len(rows) || rows[m.graphIndex].Hash == "" {
		return "", fmt.Errorf("select a commit first")
	}
	base, err := tools.GitHubURL(p, s)
	if err != nil {
		return "", err
	}
	return base + "/commit/" + rows[m.graphIndex].Hash, nil
}

func (m *Model) setGitStatus(id string, status git.Status) {
	p, ok := m.current()
	hash := ""
	if ok && p.ID == id {
		hash = m.selectedGraphHash()
	}
	m.statuses[id] = status
	if ok && p.ID == id {
		m.restoreGraphSelection(hash)
	}
}
