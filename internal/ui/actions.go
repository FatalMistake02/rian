package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"rian/internal/config"
	"rian/internal/detect"
	"rian/internal/project"
	"rian/internal/tools"
)

func (m *Model) actions(p project.Project) []Action {
	items := []Action{}
	for _, tool := range []struct{ id, label, binary string }{{"vscode", "Open in VS Code", "code"}, {"opencode", "Open in OpenCode", "opencode"}, {"kilocode", "Open in KiloCode", "kilocode"}, {"terminal", "Open Terminal", ""}} {
		item := Action{ID: tool.id, Label: tool.label}
		if !m.shownFor(p, tool.id) {
			item.Disabled = "Disabled in project settings"
		} else if tool.binary != "" && !m.available[tool.binary] {
			item.Disabled = tool.binary + " was not found in PATH"
		}
		items = append(items, item)
	}
	repoURL, err := tools.GitHubURL(p, m.statuses[p.ID])
	item := Action{ID: "github", Label: "Open GitHub", Detail: repoURL}
	if err != nil {
		item.Disabled = "No GitHub remote or override"
	}
	items = append(items, item)
	files := Action{ID: "files", Label: "Open in Files"}
	if !m.available["files"] {
		files.Disabled = "No default file manager launcher found"
	}
	items = append(items, files)
	for _, label := range []string{"local", "production"} {
		if p.URLs[label] != "" {
			items = append(items, Action{ID: "url", Label: "Open " + label + " URL", Value: p.URLs[label], Detail: p.URLs[label]})
		}
	}
	for _, name := range detect.CommandNames(m.visibleCommands(p)) {
		items = append(items, Action{ID: "run", Label: "Run " + name, Value: name, Detail: p.AllCommands()[name]})
	}
	items = append(items, m.gitActionsFor(p)...)
	items = append(items, m.addonActions(p)...)
	pin := "Pin project"
	if p.Favorite {
		pin = "Unpin project"
	}
	return append(items, Action{ID: "notes", Label: "Notes"}, Action{ID: "pin", Label: pin}, Action{ID: "edit", Label: "Edit project"}, Action{ID: "add", Label: "Add project"}, Action{ID: "refresh", Label: "Rescan project", Detail: "Refresh stack, commands, tools, and Git"}, Action{ID: "settings", Label: "Settings"}, Action{ID: "remove", Label: "Remove project"})
}

func (m *Model) gitActions() []Action { p, _ := m.current(); return m.gitActionsFor(p) }
func (m *Model) gitActionsFor(p project.Project) []Action {
	s := m.statuses[p.ID]
	if s.Error != "" {
		return nil
	}
	if !s.Repository {
		if m.gitMenuAvailable(p) {
			return []Action{{ID: "git", Label: "Initialize Git", Value: "init"}}
		}
		return nil
	}
	items := []Action{}
	if !s.Conflicted && (s.Dirty() || s.Staged > 0) {
		items = append(items, Action{ID: "commit", Label: "Commit"})
	}
	if s.Head != "" && !s.Conflicted {
		items = append(items, Action{ID: "branch", Label: "Switch branch"})
	}
	if s.Remote != "" {
		items = append(items, Action{ID: "git", Label: "Fetch", Value: "fetch"})
	}
	if s.Tracking != "" && s.Behind > 0 && s.Ahead == 0 && !s.Dirty() && !s.Conflicted {
		items = append(items, Action{ID: "git", Label: "Pull", Value: "pull"})
	}
	if s.Remote != "" && s.Head != "" && s.Attached && !s.Conflicted && s.Behind == 0 && (s.Ahead > 0 || s.Tracking == "") {
		items = append(items, Action{ID: "git", Label: "Push", Value: "push"})
	}
	items = append(items, Action{ID: "addRemote", Label: "Add remote"})
	if _, err := tools.GitHubURL(p, s); err == nil {
		items = append(items, Action{ID: "github", Label: "Open GitHub"})
	}
	if m.available["lazygit"] {
		items = append(items, Action{ID: "lazygit", Label: "Open lazygit"})
	}
	return items
}

func (m *Model) openMenu(which mode) tea.Cmd {
	m.mode, m.menuIndex = which, 0
	m.query = input("Search actions")
	p, ok := m.current()
	if m.overview {
		m.menu = []Action{{ID: "projects", Label: "Projects"}, {ID: "settings", Label: "Settings"}, {ID: "add", Label: "Add project"}}
		for _, page := range m.addonOverviewPages() {
			m.menu = append(m.menu, Action{ID: "overviewPage", Label: page.page.Name, Value: page.addonID + "/" + page.page.ID})
		}
	} else if !ok {
		m.menu = []Action{{ID: "add", Label: "Add project"}}
	} else {
		switch which {
		case gitMenu:
			return m.readGitAction(p, "menu")
		case commands:
			m.menu = nil
			for _, name := range detect.CommandNames(m.visibleCommands(p)) {
				detail := p.AllCommands()[name]
				if _, custom := p.Commands[name]; custom {
					detail = "Custom · " + detail
				}
				m.menu = append(m.menu, Action{ID: "run", Label: name, Value: name, Detail: detail})
			}
			m.menu = append(m.menu, m.addonCommandActions(p)...)
			m.menu = append(m.menu, Action{ID: "custom", Label: "+ Add custom command"}, Action{ID: "adhoc", Label: "Run a one-off command"})
		default:
			m.menu = m.actions(p)
		}
	}
	m.resize()
	return m.query.Focus()
}

func (m *Model) filteredActions() []Action {
	items := []Action{}
	for _, action := range m.menu {
		if action.Disabled == "" && (m.mode != palette || m.paletteActionAvailable(action)) && m.actionShown(action) && project.Score(m.query.Value(), action.Label) >= 0 {
			items = append(items, action)
		}
	}
	if m.mode == palette || m.query.Value() != "" {
		sort.SliceStable(items, func(i, j int) bool {
			if m.mode == palette {
				left, right := paletteGroup(items[i]), paletteGroup(items[j])
				if left != right {
					return left < right
				}
			}
			if m.query.Value() != "" {
				return project.Score(m.query.Value(), items[i].Label) > project.Score(m.query.Value(), items[j].Label)
			}
			return false
		})
	}
	seen := map[string]bool{}
	unique := []Action{}
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		key := item.ID + "\x00" + item.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, item)
	}
	for left, right := 0, len(unique)-1; left < right; left, right = left+1, right-1 {
		unique[left], unique[right] = unique[right], unique[left]
	}
	items = unique
	return items
}

func (m *Model) execute(action Action) tea.Cmd {
	if action.Disabled != "" {
		return m.notify("Action unavailable", fmt.Errorf("%s", action.Disabled))
	}
	if action.ID == "overviewPage" {
		for i, page := range m.addonOverviewPages() {
			if page.addonID+"/"+page.page.ID == action.Value {
				m.closeModal()
				m.overview = true
				m.overviewFocused = false
				m.overviewIndex = i + 2
				return nil
			}
		}
		return nil
	}
	if action.ID == "projects" {
		m.closeModal()
		m.overview, m.projectFull = false, false
		return nil
	}
	if action.ID == "settings" {
		m.startSettings()
		return nil
	}
	if !m.actionShown(action) {
		return nil
	}
	if action.ID == "add" {
		m.startAdd()
		return m.query.Focus()
	}

	p, ok := m.current()
	if !ok {
		return nil
	}
	if !m.shownFor(p, action.ID) {
		return m.notify("Action disabled", fmt.Errorf("%s is disabled in project settings", action.Label))
	}
	if strings.HasPrefix(action.ID, "addon/") || strings.HasPrefix(action.ID, "addon-action/") {
		return m.executeAddon(action, p)
	}
	m.closeModal()
	switch action.ID {
	case "pin":
		return m.save(p.ID, "Pin updated", func(cfg *config.Config) error {
			for i := range cfg.Projects {
				if cfg.Projects[i].ID == p.ID {
					cfg.Projects[i].Favorite = !cfg.Projects[i].Favorite
					return nil
				}
			}
			return fmt.Errorf("project no longer exists")
		})
	case "notes":
		return m.startNotes(p)
	case "edit":
		m.startEdit(p)
		return m.fields[0].input.Focus()
	case "remove":
		m.editProject = p
		m.mode = removeConfirm
		return nil
	case "refresh":
		return m.inspectDirectory("refresh", p.Path, p.ID)
	case "process":
		m.showProcess(p.ID)
		return nil
	case "run":
		return m.runCommand(p, action.Value, p.AllCommands()[action.Value])
	case "adhoc":
		m.startPrompt("run", "Command to run")
		return m.query.Focus()
	case "custom":
		m.startCustom("", "")
		return m.commandName.Focus()
	case "addRemote":
		return m.startRemote(p)
	case "remoteChoice":
		m.startRemoteForm(action.Value)
		return m.remoteInputs[0].Focus()
	case "commit", "branch":
		return m.readGitAction(p, action.ID)
	case "newBranch":
		m.startPrompt("newBranch", "New branch name")
		return m.query.Focus()
	case "switchBranch":
		return m.switchBranch(p, action.Value, false)
	case "git":
		args := []string{action.Value}
		if action.Value == "fetch" {
			args = append(args, "--all")
		}
		if action.Value == "pull" {
			args = append(args, "--ff-only")
		}
		if action.Value == "push" {
			s := m.statuses[p.ID]
			if s.Tracking == "" {
				args = append(args, "--set-upstream", s.RemoteName, s.Branch)
			}
		}
		return m.runGit(p, action.Label, [][]string{args})
	case "lazygit":
		return m.interactive(p, func() (*exec.Cmd, error) { return m.launcher.Interactive("lazygit", p) }, "lazygit", true)
	case "terminal", "opencode", "kilocode":
		return m.launchInteractive(action.ID, p, func() (*exec.Cmd, error) { return m.launcher.Interactive(action.ID, p) }, action.Label, true)
	default:
		launcher := m.launcher
		status := m.statuses[p.ID]
		return func() tea.Msg {
			var err error
			switch action.ID {
			case "files":
				err = launcher.OpenFiles(p)
			case "vscode":
				err = launcher.OpenEditor(p)
			case "github":
				var value string
				value, err = tools.GitHubURL(p, status)
				if err == nil {
					err = launcher.OpenURL(value)
				}
			case "url":
				err = launcher.OpenURL(action.Value)
			}
			return actionMsg{id: p.ID, notice: action.Label + " opened", err: err}
		}
	}
}

type preparedExecMsg struct {
	cmd       *exec.Cmd
	id, label string
	refresh   bool
	err       error
}

func (m *Model) launchInteractive(id string, p project.Project, prepare func() (*exec.Cmd, error), label string, refresh bool) tea.Cmd {
	mode := m.cfg.Settings.LaunchMode("cli")
	if mode == "ask" {
		return m.askLaunch(label, func(mode string) tea.Cmd { return m.launchInteractiveMode(mode, p, prepare, label, refresh) })
	}
	return m.launchInteractiveMode(mode, p, prepare, label, refresh)
}

func (m *Model) launchInteractiveMode(mode string, p project.Project, prepare func() (*exec.Cmd, error), label string, refresh bool) tea.Cmd {
	if mode == "same" {
		return m.interactive(p, prepare, label, refresh)
	}
	return func() tea.Msg {
		child, err := prepare()
		if err == nil {
			err = tools.OpenTerminal(child, mode)
		}
		return actionMsg{id: p.ID, notice: label + " opened", err: err}
	}
}

func (m *Model) interactive(p project.Project, prepare func() (*exec.Cmd, error), label string, refresh bool) tea.Cmd {
	return func() tea.Msg { cmd, err := prepare(); return preparedExecMsg{cmd, p.ID, label, refresh, err} }
}

func (m *Model) runCommand(p project.Project, name, command string) tea.Cmd {
	if strings.TrimSpace(command) == "" {
		return m.notify("No command", fmt.Errorf("this command is empty"))
	}
	mode := m.cfg.Settings.LaunchMode("run")
	if mode == "ask" {
		return m.askLaunch(name, func(mode string) tea.Cmd { return m.runCommandMode(mode, p, name, command) })
	}
	return m.runCommandMode(mode, p, name, command)
}

func (m *Model) runCommandMode(mode string, p project.Project, name, command string) tea.Cmd {
	return m.launchInteractiveMode(mode, p, func() (*exec.Cmd, error) { return tools.Shell(p.Path, command), nil }, name, false)
}

func (m *Model) showProcess(id string) {
	m.mode = processView
	m.resize()
	m.processID = id
	m.processState = m.manager.Snapshot(id)
	m.followOutput = true
	m.viewport.SetContent(plain(m.processState.Output))
	m.viewport.GotoBottom()
}

func (m *Model) startPrompt(action, placeholder string) {
	m.mode = prompt
	m.promptAction = action
	m.query = input(placeholder)
	m.resize()
}

func (m *Model) submitPrompt() tea.Cmd {
	value := strings.TrimSpace(m.query.Value())
	if value == "" {
		m.formError = "Enter a value first"
		return nil
	}
	p, ok := m.current()
	if !ok {
		return nil
	}
	action := m.promptAction
	m.closeModal()
	if action == "run" {
		return m.runCommand(p, "custom", value)
	}
	if action == "commit" {
		steps := [][]string{}
		if m.commitAutoStage {
			steps = append(steps, []string{"add", "--all"})
		}
		steps = append(steps, []string{"commit", "-m", value})
		return m.runGit(p, "Commit", steps)
	}
	return m.switchBranch(p, value, action == "newBranch")
}
