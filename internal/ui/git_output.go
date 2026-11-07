package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/git"
	"rian/internal/process"
	"rian/internal/project"
	"rian/internal/tools"
)

type gitRunMsg struct {
	job     string
	state   process.Snapshot
	err     error
	skipped bool
}

func (m *Model) runGit(p project.Project, title string, steps [][]string) tea.Cmd {
	m.outputDone = nil
	m.outputPrepare = nil
	m.mode = gitOutput
	m.gitProject, m.gitTitle, m.gitSteps, m.gitStep = p, title, steps, 0
	m.gitLog, m.gitError, m.gitRunning = "", "", true
	m.resize()
	m.viewport.SetContent("Starting Git")
	m.viewport.GotoTop()
	return m.startGitStep()
}

func (m *Model) startGitStep() tea.Cmd {
	m.gitJobStarted = false
	m.gitJobID = fmt.Sprintf("git:%s:%d", m.gitProject.ID, time.Now().UnixNano())
	job, p, args, manager := m.gitJobID, m.gitProject, m.gitSteps[m.gitStep], m.manager
	m.gitLog += "$ git " + oneLine(strings.Join(args, " ")) + "\n"
	m.viewport.SetContent(m.gitLog)
	return func() tea.Msg {
		if args[0] == "add" {
			status := git.Inspect(p.Path)
			if status.Error != "" || !status.Repository {
				return gitRunMsg{job: job, err: fmt.Errorf("unable to inspect staging area: %s", status.Error)}
			}
			if status.Staged > 0 {
				return gitRunMsg{job: job, skipped: true}
			}
		}
		cmd, err := tools.GitCommand(p, args...)
		if err != nil {
			return gitRunMsg{job: job, err: err}
		}
		// Git commands report failures instead of waiting for an invisible prompt.
		cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat")
		state, err := manager.StartCommand(job, "git "+strings.Join(args, " "), cmd)
		return gitRunMsg{job: job, state: state, err: err}
	}
}

func (m *Model) gitProgress(msg gitRunMsg) tea.Cmd {
	if m.mode != gitOutput || msg.job != m.gitJobID {
		return nil
	}
	m.gitJobStarted = true
	output := plain(msg.state.Output)
	follow := m.viewport.AtBottom()
	m.viewport.SetContent(m.gitLog + output)
	if follow {
		m.viewport.GotoBottom()
	}
	if msg.err != nil {
		m.gitError = msg.err.Error()
	} else {
		m.gitError = msg.state.Error
	}
	if msg.state.Running && m.gitError == "" {
		return nil
	}
	m.manager.Forget(m.gitJobID)
	m.gitLog += output
	if m.gitError != "" {
		m.gitRunning = false
		m.viewport.SetContent(m.gitLog + "\n" + warning.Render(oneLine(m.gitError)))
		m.viewport.GotoBottom()
		return m.refresh(m.gitProject, true)
	}
	if msg.skipped {
		m.gitLog += "Using files that are already staged.\n"
	}
	m.gitStep++
	if m.gitStep < len(m.gitSteps) {
		return m.startGitStep()
	}
	m.gitRunning = false
	if m.outputDone != nil {
		done := m.outputDone
		m.outputDone = nil
		return done()
	}
	m.closeModal()
	return tea.Batch(m.refresh(m.gitProject, true), m.notify(m.gitTitle+" completed", nil))
}
