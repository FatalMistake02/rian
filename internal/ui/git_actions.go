package ui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"rian/internal/git"
	"rian/internal/project"
)

type gitActionMsg struct {
	id, action string
	status     git.Status
}

func (m *Model) readGitAction(p project.Project, action string) tea.Cmd {
	m.busy = true
	m.mode = gitMenu
	m.menu = nil
	m.query = input("Search actions")
	return func() tea.Msg { return gitActionMsg{p.ID, action, git.Inspect(p.Path)} }
}

func (m *Model) finishGitAction(msg gitActionMsg) tea.Cmd {
	m.busy = false
	if m.mode != gitMenu {
		return nil
	}
	p, ok := m.current()
	if !ok || p.ID != msg.id {
		return nil
	}
	m.setGitStatus(p.ID, msg.status)
	if msg.status.Error != "" || !msg.status.Repository && msg.action != "menu" {
		m.closeModal()
		message := msg.status.Error
		if message == "" {
			message = "No Git repository detected"
		}
		return m.notify("Unable to read Git", fmt.Errorf("%s", message))
	}
	if msg.action == "menu" {
		m.mode = gitMenu
		m.menu = m.gitActionsFor(p)
		m.menuIndex = 0
		m.query = input("Search actions")
		m.resize()
		return m.query.Focus()
	}
	if msg.action == "branch" {
		m.mode, m.menuIndex = branchMenu, 0
		m.menu = []Action{{ID: "newBranch", Label: "+ Create new branch"}}
		for _, name := range msg.status.Branches {
			label := name
			if name == msg.status.Branch {
				continue
			}
			m.menu = append(m.menu, Action{ID: "switchBranch", Label: label, Value: name})
		}
		m.query = input("Search branches")
		m.resize()
		return m.query.Focus()
	}
	m.commitAutoStage = false
	if msg.status.Staged == 0 {
		if !msg.status.Dirty() {
			m.closeModal()
			return m.notify("Nothing to commit", nil)
		}
		m.mode = stageConfirm
		m.resize()
		return nil
	}
	m.startPrompt("commit", "Commit message")
	return m.query.Focus()
}

func (m *Model) switchBranch(p project.Project, name string, create bool) tea.Cmd {
	steps := [][]string{{"switch", "--", name}}
	if create {
		steps = [][]string{{"check-ref-format", "--branch", name}, {"switch", "-c", name}}
	}
	return m.runGit(p, "Switch branch", steps)
}

func (m *Model) gitMenuAvailable(p project.Project) bool {
	status := m.statuses[p.ID]
	if status.Error != "" || !(m.available["git"] || status.Repository) {
		return false
	}
	info, err := os.Stat(p.Path)
	return err == nil && info.IsDir()
}
