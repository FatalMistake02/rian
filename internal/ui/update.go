package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"rian/internal/config"
	"rian/internal/project"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case addonUpdatesMsg:
		return m, m.acceptAddonUpdates(msg)
	case addonUpdatedMsg:
		return m, m.acceptAddonUpdate(msg)
	case releaseMsg:
		if msg.err == nil {
			m.latestRelease = msg.version
		}
	case updatedMsg:
		m.updating = false
		if msg.err != nil {
			return m, m.notify("Update failed", msg.err)
		}
		m.latestRelease = ""
		return m, m.notify("Updated. Restart Rian to use the new version.", nil)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case readyMsg:
		m.available = msg.available
		if msg.err != nil {
			return m, m.notify("Unable to load projects", msg.err)
		}
		m.cfg = msg.cfg
		m.applyAppearance()
		m.overview = m.cfg.Settings.StartPage() == "overview"
		m.overviewFocused = false
		m.projectFull, m.graphFocused = false, false
		m.openStartupProject()
		cmds := []tea.Cmd{m.scanAutoAdd(), m.checkAddonUpdates()}
		// Initial badges are loaded in small parallel batches; subsequent polls
		// inspect only the active project.
		for i, p := range m.cfg.Projects {
			if i >= 8 {
				break
			}
			cmds = append(cmds, m.refresh(p, true))
		}
		return m, tea.Batch(cmds...)
	case autoAddSkippedMsg:
		m.busy = false
		if msg.err != nil {
			return m, m.notify("Could not remember skipped folder", msg.err)
		}
		if msg.cfg.Revision >= m.cfg.Revision {
			m.cfg = msg.cfg
		}
		m.autoAddReviewIndex = min(m.autoAddReviewIndex, max(0, len(m.pendingAutoAdd())-1))
	case autoAddScanMsg:
		m.autoAddScanning = false
		if msg.key != autoAddKey(m.cfg.Settings) {
			return m, m.scanAutoAdd()
		}
		if msg.err != nil {
			m.autoAddCandidates = nil
			if msg.err.Error() != m.autoAddScanError {
				m.autoAddScanError = msg.err.Error()
				return m, m.notify("Auto add unavailable", msg.err)
			}
			return m, nil
		}
		m.autoAddScanError = ""
		m.autoAddCandidates = msg.projects
		m.autoAddReviewIndex = min(m.autoAddReviewIndex, max(0, len(m.pendingAutoAdd())-1))
	case gitRunMsg:
		return m, m.gitProgress(msg)
	case remoteReadyMsg:
		return m, m.finishRemote(msg)
	case gitActionMsg:
		return m, m.finishGitAction(msg)
	case gitMsg:
		m.setGitStatus(msg.id, msg.status)
		m.loading[msg.id], m.lastRefresh[msg.id] = false, time.Now()
	case pickedFolderMsg:
		m.busy = false
		if m.mode != add && m.mode != githubAdd && m.mode != autoAddLocation {
			return m, nil
		}
		if msg.err != nil {
			m.formError = msg.err.Error()
			return m, m.query.Focus()
		}
		if msg.path == "" {
			m.formError = ""
			return m, m.query.Focus()
		}
		m.query.SetValue(msg.path)
		if m.mode == autoAddLocation {
			return m, m.setAutoAddLocation(msg.path)
		}
		if m.mode == githubAdd {
			return m, m.githubChooseLocation(msg.path)
		}
		return m, m.inspectDirectory("add", msg.path, "")
	case inspectedMsg:
		m.busy = false
		if msg.purpose == "add" && m.mode != add || msg.purpose == "edit" && m.mode != edit {
			return m, nil
		}
		if msg.err != nil {
			m.formError = msg.err.Error()
			if m.autoAddReviewing && msg.purpose == "add" {
				m.mode = autoAddReview
			}
			return m, m.notify("Could not inspect directory", msg.err)
		}
		m.available = msg.result.Available
		if msg.purpose == "add" {
			for _, p := range m.cfg.Projects {
				if project.SamePath(p.Path, msg.result.Project.Path) {
					m.formError = "Already saved as " + p.Name
					return m, nil
				}
			}
			m.pending, m.mode = msg.result, addConfirm
			m.viewport.SetContent(m.detectionSummary(msg.result))
			m.viewport.GotoTop()
		} else if msg.purpose == "edit" {
			p := m.editProject
			p.Path, p.AutoCommands, p.Stack = msg.result.Project.Path, msg.result.Project.AutoCommands, msg.result.Project.Stack
			m.statuses[p.ID] = msg.result.Git
			return m, m.save(p.ID, "Project updated", func(cfg *config.Config) error { return config.Replace(cfg, p) })
		} else {
			m.statuses[msg.id] = msg.result.Git
			return m, m.save(msg.id, "Project rescanned", func(cfg *config.Config) error {
				for i := range cfg.Projects {
					if cfg.Projects[i].ID == msg.id {
						cfg.Projects[i].AutoCommands = msg.result.Project.AutoCommands
						cfg.Projects[i].Stack = msg.result.Project.Stack
						return nil
					}
				}
				return fmt.Errorf("project no longer exists")
			})
		}
	case savedMsg:
		// A recent-use write can finish while an edit is still saving. Only
		// the explicit mutation that set busy may release that input guard.
		if msg.notice != "" {
			m.busy = false
		}
		if msg.err != nil {
			m.formError = msg.err.Error()
			return m, m.notify("Could not save project", msg.err)
		}
		selectedID := msg.id
		if msg.notice == "Project added" {
			m.overview = false
		}
		if msg.notice == "Project added" || msg.notice == "Project updated" {
			m.search.Reset()
			m.searching = false
			m.search.Blur()
		}
		if msg.notice == "" {
			if p, ok := m.current(); ok {
				selectedID = p.ID
			}
		}
		if msg.cfg.Revision >= m.cfg.Revision {
			m.cfg = msg.cfg
			if (m.mode == autoAddLocation || m.mode == settings || m.mode == addonImport || m.mode == addonEdit || m.mode == addonRemove) && msg.notice != "Settings saved" {
				m.cfg.Settings = m.settingsDraft.Clone()
			}
			m.applyAppearance()
		}
		m.selectID(selectedID)
		if msg.notice != "" {
			m.closeModal()
			if msg.notice == "Project added" && m.autoAddReviewing {
				m.mode = autoAddReview
				m.autoAddReviewIndex = min(m.autoAddReviewIndex, max(0, len(m.pendingAutoAdd())-1))
			}
			return m, m.notify(msg.notice, nil)
		}
	case addonImportedMsg:
		return m, m.acceptAddon(msg)
	case preparedExecMsg:
		if msg.err != nil {
			return m, m.notify(msg.label, msg.err)
		}
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			if err != nil {
				err = fmt.Errorf("%s failed: %w", msg.label, err)
			}
			return actionMsg{msg.id, msg.label + " finished", err, msg.refresh}
		})
	case actionMsg:
		cmds := []tea.Cmd{m.notify(msg.notice, msg.err)}
		if msg.err == nil && msg.id != "" {
			store := m.store
			cmds = append(cmds, func() tea.Msg {
				cfg, err := store.Update(func(cfg *config.Config) error { return config.Touch(cfg, msg.id) })
				return savedMsg{cfg: cfg, id: msg.id, err: err}
			})
		}
		if msg.refresh {
			for _, p := range m.cfg.Projects {
				if p.ID == msg.id {
					cmds = append(cmds, m.refresh(p, true))
					break
				}
			}
		}
		return m, tea.Batch(cmds...)
	case processMsg:
		m.busy = false
		if msg.err != nil {
			return m, m.notify("Command failed", msg.err)
		}
		if m.processID == msg.id {
			m.processState = msg.state
		}
		store := m.store
		return m, func() tea.Msg {
			cfg, err := store.Update(func(cfg *config.Config) error { return config.Touch(cfg, msg.id) })
			return savedMsg{cfg: cfg, id: msg.id, err: err}
		}
	case tickMsg:
		m.autoAddBlink = !m.autoAddBlink
		cmds := []tea.Cmd{tick(), m.scanAutoAdd(), m.checkAddonUpdates()}
		if m.mode == gitOutput && m.gitRunning && m.gitJobStarted {
			cmds = append(cmds, m.gitProgress(gitRunMsg{job: m.gitJobID, state: m.manager.Snapshot(m.gitJobID)}))
		}
		if m.overview {
			inFlight := 0
			for _, loading := range m.loading {
				if loading {
					inFlight++
				}
			}
			for _, p := range m.cfg.Projects {
				if inFlight >= 8 {
					break
				}
				if cmd := m.refresh(p, false); cmd != nil {
					cmds = append(cmds, cmd)
					inFlight++
				}
			}
		} else if p, ok := m.current(); ok {
			cmds = append(cmds, m.refresh(p, false))
		}
		if m.mode == processView {
			m.processState = m.manager.Snapshot(m.processID)
			m.viewport.SetContent(plain(m.processState.Output))
			if m.followOutput {
				m.viewport.GotoBottom()
			}
		}
		return m, tea.Batch(cmds...)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case toastExpired:
		if int(msg) == m.toastID {
			m.toast = ""
		}
	case tea.FocusMsg:
		if p, ok := m.current(); ok {
			return m, m.refresh(p, true)
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	default:
		var cmd tea.Cmd
		if m.mode == notes {
			m.notesInput, cmd = m.notesInput.Update(msg)
		} else if m.mode == edit && len(m.fields) > 0 {
			m.fields[m.fieldIndex].input, cmd = m.fields[m.fieldIndex].input.Update(msg)
		} else if m.commandEditing {
			if m.commandField == 0 {
				m.commandName, cmd = m.commandName.Update(msg)
			} else {
				m.commandValue, cmd = m.commandValue.Update(msg)
			}
		} else if m.mode == remoteForm {
			m.remoteInputs[m.remoteField], cmd = m.remoteInputs[m.remoteField].Update(msg)
		} else if m.mode == settings {
			m.settingsSearch, cmd = m.settingsSearch.Update(msg)
		} else if m.searching && m.mode == home {
			m.search, cmd = m.search.Update(msg)
		} else {
			m.query, cmd = m.query.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" {
		if m.mode == autoAddLocation || m.mode == settings || m.mode == addonImport || m.mode == addonEdit || m.mode == addonRemove {
			return m.settingsKey("ctrl+c")
		}
		return m.quit()
	}
	if m.busy {
		if key == "esc" {
			m.closeModal()
		}
		return nil
	}
	if m.mode == home && m.searching {
		if key == "backspace" && m.search.Value() == "" {
			key = "esc"
		}
		switch key {
		case "esc":
			m.searching = false
			m.search.Reset()
			m.search.Blur()
			m.selected = 0
			return nil
		case "enter":
			m.searching = false
			m.search.Blur()
			if _, ok := m.current(); ok {
				m.projectFull = true
			}
			return nil
		case "up", "down":
			return m.navigate(key)
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			return cmd
		}
	}
	if m.mode != home {
		return m.modalKey(msg)
	}
	if key == "u" && m.latestRelease != "" && !m.updating {
		m.mode, m.menuIndex = updateMenu, 0
		return nil
	}
	if (key == "U" || key == "shift+u") && len(m.pendingAddonUpdates()) > 0 {
		m.mode, m.menuIndex = addonUpdates, 0
		m.resize()
		return nil
	}
	if (key == "A" || key == "shift+a") && len(m.pendingAutoAdd()) > 0 {
		m.openAutoAddReview()
		return nil
	}
	if m.overview {
		if handled, cmd := m.overviewKey(key); handled {
			return cmd
		}
	}
	if m.projectFull {
		if handled, cmd := m.notesPaneKey(key); handled {
			return cmd
		}
		if handled, cmd := m.graphKey(key); handled {
			return cmd
		}
	}
	if !m.overview && !m.graphFocused {
		if handled, cmd := m.addonShortcut(key); handled {
			return cmd
		}
	}
	switch key {
	case "q":
		return m.quit()
	case "up", "down", "j", "k", "home", "end":
		if m.projectFull {
			return nil
		}
		return m.navigate(key)
	case "/":
		if m.projectFull {
			return nil
		}
		m.searching = true
		return m.search.Focus()
	case "esc":
		if m.projectFull {
			m.projectFull = false
			m.graphFocused = false
			return nil
		}
		m.search.Reset()
		m.selected = 0
		m.overview = true
	case "ctrl+k":
		return m.openMenu(palette)
	case "s":
		m.startSettings()
		return nil
	case "a", "+":
		m.startAdd()
		return m.query.Focus()
	case "enter":
		if _, ok := m.current(); ok {
			m.projectFull = true
			m.graphFocused, m.graphIndex = false, 0
			m.notesFocused = false
			m.notesOffset = 0
			m.restoreGraphSelection("")
			return nil
		}
	case "n":
		return m.execute(Action{ID: "notes", Label: "Notes"})
	case "f":
		return m.execute(Action{ID: "files", Label: "Files"})
	case "v":
		return m.execute(Action{ID: "vscode", Label: "VS Code"})
	case "o":
		return m.execute(Action{ID: "opencode", Label: "OpenCode"})
	case "K", "shift+k":
		return m.execute(Action{ID: "kilocode", Label: "KiloCode"})
	case "t":
		return m.execute(Action{ID: "terminal", Label: "Terminal"})
	case "g":
		return m.execute(Action{ID: "github", Label: "GitHub"})
	case "G", "shift+g":
		return m.openMenu(gitMenu)
	case "r":
		return m.openMenu(commands)
	case "p":
		return m.execute(Action{ID: "pin"})
	case "e":
		return m.execute(Action{ID: "edit"})
	case "delete":
		return m.execute(Action{ID: "remove"})
	case "ctrl+r":
		if _, ok := m.current(); ok {
			return m.execute(Action{ID: "refresh"})
		}
	}
	return nil
}

func (m *Model) navigate(key string) tea.Cmd {
	count := len(m.projects())
	if count == 0 {
		return nil
	}
	switch key {
	case "up", "j":
		m.selected = max(0, m.selected-1)
	case "down", "k":
		m.selected = min(count-1, m.selected+1)
	case "home":
		m.selected = 0
	case "end":
		m.selected = count - 1
	}
	p, _ := m.current()
	return m.refresh(p, false)
}

func (m *Model) modalKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if m.mode == addonUpdates || m.mode == addonUpdateConfirm {
		return m.addonUpdatesKey(key)
	}
	if m.mode == notes {
		return m.notesKey(msg)
	}
	if m.mode == updateMenu {
		return m.updateMenuKey(key)
	}
	if m.mode == autoAddLocation {
		return m.autoAddLocationKey(msg)
	}
	if m.mode == autoAddReview {
		return m.autoAddReviewKey(msg)
	}
	if m.mode == githubAdd {
		return m.githubAddKey(msg)
	}
	if m.mode == remoteForm || m.mode == remoteConfirm {
		return m.remoteFormKey(msg)
	}
	if m.mode == addonImport || m.mode == addonEdit || m.mode == addonRemove {
		return m.addonFormKey(msg)
	}
	if m.mode == settings {
		return m.settingsInput(msg)
	}
	if key == "esc" {
		if m.mode == gitOutput && m.gitRunning {
			return nil
		}
		if m.autoAddReviewing && (m.mode == add || m.mode == addConfirm) {
			m.mode = autoAddReview
			m.formError = ""
			return nil
		}
		m.closeModal()
		return nil
	}
	switch m.mode {
	case gitOutput:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return cmd
	case launchChoice:
		return m.launchChoiceKey(key)
	case settings:
		return m.settingsKey(key)
	case add:
		if key == "g" && m.query.Value() == "" {
			return m.startGitHubAdd()
		}
		if key == "enter" {
			return m.submitAdd()
		}
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		return cmd
	case addConfirm:
		if key == "enter" {
			m.statuses[m.pending.Project.ID] = m.pending.Git
			return m.confirmAdd()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return cmd
	case edit:
		switch key {
		case "ctrl+s":
			return m.submitEdit()
		case "tab", "down", "enter":
			if key == "enter" && m.fields[m.fieldIndex].toggle {
				m.fields[m.fieldIndex].checked = !m.fields[m.fieldIndex].checked
				return nil
			}
			return m.focusField(m.fieldIndex + 1)
		case "shift+tab", "up":
			return m.focusField(m.fieldIndex - 1)
		case " ", "space":
			if m.fields[m.fieldIndex].toggle {
				m.fields[m.fieldIndex].checked = !m.fields[m.fieldIndex].checked
				return nil
			}
		}
		var cmd tea.Cmd
		m.fields[m.fieldIndex].input, cmd = m.fields[m.fieldIndex].input.Update(msg)
		return cmd
	case palette, commands, gitMenu, branchMenu, addonMenu, remoteMenu:
		items := m.filteredActions()
		switch key {
		case "up", "ctrl+p":
			m.menuIndex = max(0, m.menuIndex-1)
		case "down", "ctrl+n":
			if key == "ctrl+n" && m.mode == commands {
				m.startCustom("", "")
				return m.commandName.Focus()
			}
			m.menuIndex = min(max(0, len(items)-1), m.menuIndex+1)
		case "enter":
			if len(items) > 0 {
				return m.execute(items[min(m.menuIndex, len(items)-1)])
			}
		case "ctrl+e":
			if m.mode == commands {
				return m.commandEdit(false)
			}
		case "ctrl+d":
			if m.mode == commands {
				return m.commandEdit(true)
			}
		default:
			var cmd tea.Cmd
			m.query, cmd = m.query.Update(msg)
			m.menuIndex = 0
			return cmd
		}
	case stageConfirm:
		if key == "enter" || key == "y" {
			m.commitAutoStage = true
			m.startPrompt("commit", "Commit message")
			return m.query.Focus()
		}
		if key == "n" {
			m.closeModal()
		}
	case removeConfirm:
		if key == "enter" || key == "y" {
			p := m.editProject
			if m.manager.Snapshot(p.ID).Running {
				return m.notify("Command running", fmt.Errorf("stop this project's command before removing it"))
			}
			return m.save("", "Project removed from Rian", func(cfg *config.Config) error { return config.Remove(cfg, p.ID) })
		}
		if key == "n" {
			m.closeModal()
		}
	case quitConfirm:
		if key == "enter" || key == "y" {
			manager := m.manager
			return tea.Sequence(func() tea.Msg { manager.Close(); return nil }, tea.Quit)
		}
		if key == "n" {
			m.closeModal()
		}
	case prompt:
		if m.commandEditing {
			if key == "ctrl+s" {
				return m.submitCustom()
			}
			if key == "tab" || key == "shift+tab" || key == "enter" {
				if m.commandField == 0 {
					m.commandName.Blur()
					m.commandField = 1
					return m.commandValue.Focus()
				}
				if key == "enter" {
					return m.submitCustom()
				}
				m.commandValue.Blur()
				m.commandField = 0
				return m.commandName.Focus()
			}
			var cmd tea.Cmd
			if m.commandField == 0 {
				m.commandName, cmd = m.commandName.Update(msg)
			} else {
				m.commandValue, cmd = m.commandValue.Update(msg)
			}
			return cmd
		}
		if key == "enter" {
			return m.submitPrompt()
		}
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		return cmd
	case processView:
		if key == "s" && m.processState.Running {
			manager, id := m.manager, m.processID
			return func() tea.Msg { err := manager.Stop(id); return actionMsg{notice: "Process stopped", err: err} }
		}
		if key == "r" && !m.processState.Running && m.processState.Command != "" {
			p, ok := m.current()
			if ok {
				return m.runCommand(p, "", m.processState.Command)
			}
		}
		if key == "f" {
			m.followOutput = !m.followOutput
			if m.followOutput {
				m.viewport.GotoBottom()
			}
			return nil
		}
		if key == "up" || key == "j" || key == "pgup" || key == "home" {
			m.followOutput = false
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return cmd
	}
	return nil
}
