package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/addon"
	"rian/internal/project"
	"rian/internal/tools"
)

func (m *Model) addonSettingsRows() {
	m.settingsRows = append(m.settingsRows, settingRow{id: "addon-add", label: "+ Add addon", section: "addons", operation: "add"})
	for _, id := range addon.IDs(m.settingsDraft.Addons) {
		item := m.settingsDraft.Addons[id]
		page := "addons/" + id
		m.settingsRows = append(m.settingsRows, settingRow{id: page, label: item.Definition.Name, section: "addons", page: true, addonID: id})
		for _, entry := range []struct{ op, label string }{{"enabled", "Enabled"}, {"shortcut", "Shortcut"}} {
			m.settingsRows = append(m.settingsRows, settingRow{id: page + "/" + entry.op, label: entry.label, section: page, addonID: id, operation: entry.op})
		}
		for _, pageDef := range item.Definition.Pages {
			m.settingsRows = append(m.settingsRows, settingRow{id: "addon-page/" + id + "/" + pageDef.ID, label: pageDef.Name + " page", section: page, addonID: id, operation: "page"})
		}
		for _, s := range item.Definition.Settings {
			m.settingsRows = append(m.settingsRows, settingRow{id: page + "/" + s.ID, label: s.Name, section: page, addonID: id, addonSetting: s.ID, operation: "value"})
		}
		m.settingsRows = append(m.settingsRows, settingRow{id: page + "/remove", label: "Remove addon", section: page, addonID: id, operation: "remove"})
		m.settingsRows = append(m.settingsRows, settingRow{id: "addon/" + id, label: item.Definition.Name, section: "shortcuts"})
		for _, p := range m.cfg.Projects {
			m.settingsRows = append(m.settingsRows, settingRow{id: "addon/" + id, label: item.Definition.Name, section: "project settings/" + p.ID, projectID: p.ID})
		}
	}
}
func (m *Model) addonSettingValue(row settingRow) string {
	item := m.settingsDraft.Addons[row.addonID]
	switch row.operation {
	case "page":
		if m.settingsDraft.Shown(row.id) {
			return "Show"
		}
		return "Hide"
	case "add", "remove":
		return "›"
	case "enabled":
		if item.Enabled {
			return "Yes"
		}
		return "No"
	case "shortcut":
		if item.Definition.Shortcut == "" {
			return "None"
		}
		if m.addonKey(row.addonID) == "" {
			return item.Definition.Shortcut + " (in use)"
		}
		return item.Definition.Shortcut
	case "value":
		for _, s := range item.Definition.Settings {
			if s.ID == row.addonSetting {
				v := item.Value(s)
				if s.Type == "bool" {
					if v == "true" {
						return "Yes"
					}
					return "No"
				}
				return v
			}
		}
	}
	return ""
}
func (m *Model) changeAddonSetting(row settingRow, key string) tea.Cmd {
	item := m.settingsDraft.Addons[row.addonID]
	m.addonRow = row
	switch row.operation {
	case "add":
		m.mode = addonImport
		m.query = input("JSON file path · leave empty to select")
		m.addonLoading = false
		m.resize()
		return m.query.Focus()
	case "remove":
		m.mode = addonRemove
		return nil
	case "page":
		m.settingsDraft.Hidden[row.id] = !m.settingsDraft.Hidden[row.id]
		m.applySettingsChange()
		return nil
	case "enabled":
		item.Enabled = !item.Enabled
	case "shortcut":
		m.mode = addonEdit
		m.query = input("One letter · leave empty for no shortcut")
		m.query.SetValue(item.Definition.Shortcut)
		m.resize()
		return m.query.Focus()
	case "value":
		for _, s := range item.Definition.Settings {
			if s.ID != row.addonSetting {
				continue
			}
			v := item.Value(s)
			switch s.Type {
			case "bool":
				if v == "true" {
					v = "false"
				} else {
					v = "true"
				}
			case "choice":
				idx := 0
				for j, option := range s.Options {
					if option == v {
						idx = j
					}
				}
				step := 1
				if key == "left" {
					step = -1
				}
				v = s.Options[(idx+step+len(s.Options))%len(s.Options)]
			default:
				m.mode = addonEdit
				m.query = input(s.Name)
				m.query.SetValue(v)
				m.resize()
				return m.query.Focus()
			}
			if item.Values == nil {
				item.Values = map[string]string{}
			}
			item.Values[s.ID] = v
		}
	}
	m.settingsDraft.Addons[row.addonID] = item
	m.applySettingsChange()
	return nil
}

type addonImportedMsg struct {
	item     addon.Installed
	err      error
	canceled bool
}

func (m *Model) acceptAddon(msg addonImportedMsg) tea.Cmd {
	if m.mode != addonImport {
		return nil
	}
	m.addonLoading = false
	if msg.err != nil {
		return tea.Batch(m.query.Focus(), m.notify("Could not add addon", msg.err))
	}
	if msg.canceled {
		m.resize()
		return m.query.Focus()
	}
	id := msg.item.Definition.ID
	notice := "Addon added"
	if installed, exists := m.settingsDraft.Addons[id]; exists {
		upgraded, err := installed.Upgrade(msg.item.Definition)
		if err != nil {
			return tea.Batch(m.query.Focus(), m.notify("Could not update addon", err))
		}
		msg.item = upgraded
		notice = "Addon updated"
	}
	if m.settingsDraft.Addons == nil {
		m.settingsDraft.Addons = map[string]addon.Installed{}
	}
	m.settingsDraft.Addons[id] = msg.item
	m.applySettingsChange()
	m.buildSettingsRows()
	m.mode = settings
	m.openSettingsPage("addons/" + id)
	return m.notify(notice, nil)
}
func (m *Model) addonFormKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "esc" {
		m.mode = settings
		m.addonLoading = false
		return nil
	}
	if m.addonLoading {
		return nil
	}
	if key == "enter" {
		if m.mode == addonRemove {
			id := m.addonRow.addonID
			delete(m.settingsDraft.Addons, id)
			delete(m.settingsDraft.Hidden, "addon/"+id)
			for _, values := range m.settingsDraft.Overrides {
				delete(values, "addon/"+id)
			}
			m.applySettingsChange()
			m.buildSettingsRows()
			m.mode = settings
			m.openSettingsPage("addons")
			return nil
		}
		if m.mode == addonImport {
			path := strings.TrimSpace(m.query.Value())
			if strings.HasPrefix(path, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					path = filepath.Join(home, path[2:])
				}
			}
			m.addonLoading = true
			return func() tea.Msg {
				if path == "" {
					var err error
					path, err = tools.PickAddonFile()
					if err != nil {
						return addonImportedMsg{err: err}
					}
					if path == "" {
						return addonImportedMsg{canceled: true}
					}
				}
				item, err := addon.Read(path)
				return addonImportedMsg{item: item, err: err}
			}
		}
		row := m.addonRow
		item := m.settingsDraft.Addons[row.addonID]
		v := m.query.Value()
		if row.operation == "shortcut" {
			if !addon.ValidShortcut(v) {
				return m.notify("Invalid shortcut", fmt.Errorf("use one letter or leave empty"))
			}
			if v != "" {
				if reason := m.addonKeyConflict(row.addonID, v); reason != "" {
					return m.notify("Shortcut in use", fmt.Errorf("%s", reason))
				}
			}
			item.Definition.Shortcut = v
		} else {
			for _, s := range item.Definition.Settings {
				if s.ID == row.addonSetting {
					if err := s.ValidateValue(v); err != nil {
						return m.notify("Invalid setting", err)
					}
				}
			}
			if item.Values == nil {
				item.Values = map[string]string{}
			}
			item.Values[row.addonSetting] = v
		}
		m.settingsDraft.Addons[row.addonID] = item
		m.applySettingsChange()
		m.mode = settings
		return nil
	}
	if m.mode == addonRemove {
		return nil
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	return cmd
}
func (m *Model) addonFormView() (string, string, string) {
	footer := keyHint("Esc", "back") + "  " + keyHint("Enter", "save") + "\n" + keyHint("Ctrl+C", "cancel settings")
	switch m.mode {
	case addonImport:
		body := m.query.View()
		if m.addonLoading {
			body = "Selecting addon…"
		}
		return "ADD ADDON", body, footer
	case addonRemove:
		item := m.settingsDraft.Addons[m.addonRow.addonID]
		return "REMOVE ADDON", "Remove " + oneLine(item.Definition.Name) + "?", keyHint("Esc", "back") + "  " + keyHint("Enter", "remove") + "\n" + keyHint("Ctrl+C", "cancel settings")
	default:
		return strings.ToUpper(m.addonRow.label), m.query.View(), footer
	}
}
func (m *Model) addonKeyConflict(id, key string) string {
	if strings.Contains("abefgjknopqrstuvAGKU", key) {
		return "Rian already uses this key"
	}
	for _, other := range addon.IDs(m.cfg.Settings.Addons) {
		if other != id && m.cfg.Settings.Addons[other].Definition.Shortcut == key {
			return "Assigned to " + m.cfg.Settings.Addons[other].Definition.Name
		}
	}
	return ""
}
func (m *Model) addonKey(id string) string {
	item := m.cfg.Settings.Addons[id]
	key := item.Definition.Shortcut
	if key == "" || m.addonKeyConflict(id, key) != "" {
		return ""
	}
	return key
}
func (m *Model) addonActions(p project.Project) []Action {
	items := []Action{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		item := m.cfg.Settings.Addons[id]
		if len(item.Actions(p)) > 0 && m.shownFor(p, "addon/"+id) {
			items = append(items, Action{ID: "addon/" + id, Label: item.Definition.Name, Detail: item.Definition.Description})
		}
	}
	return items
}
func (m *Model) addonCommandActions(p project.Project) []Action {
	items := []Action{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		item := m.cfg.Settings.Addons[id]
		if !m.shownFor(p, "addon/"+id) {
			continue
		}
		for _, action := range item.Actions(p) {
			if action.Kind == "command" || action.Kind == "output" {
				items = append(items, Action{ID: "addon-action/" + id + "/" + action.ID, Label: item.Definition.Name + " / " + action.Name})
			}
		}
	}
	return items
}

func (m *Model) addonShortcut(key string) (bool, tea.Cmd) {
	p, ok := m.current()
	if !ok {
		return false, nil
	}
	for _, a := range m.addonActions(p) {
		if m.addonKey(strings.TrimPrefix(a.ID, "addon/")) == key {
			return true, m.executeAddon(a, p)
		}
	}
	return false, nil
}
func (m *Model) executeAddon(a Action, p project.Project) tea.Cmd {
	parts := strings.Split(a.ID, "/")
	if len(parts) < 2 {
		return nil
	}
	id := parts[1]
	item, ok := m.cfg.Settings.Addons[id]
	if !ok || !m.shownFor(p, "addon/"+id) {
		return nil
	}
	actions := item.Actions(p)
	if parts[0] == "addon" {
		if len(actions) == 1 && !item.Definition.Menu {
			return m.executeAddon(Action{ID: "addon-action/" + id + "/" + actions[0].ID, Label: actions[0].Name}, p)
		}
		m.mode = addonMenu
		m.addonTitle = item.Definition.Name + " · " + item.Definition.Version
		m.menu = nil
		m.menuIndex = 0
		m.query = input("Search actions")
		for _, act := range actions {
			m.menu = append(m.menu, Action{ID: "addon-action/" + id + "/" + act.ID, Label: act.Name})
		}
		m.resize()
		return m.query.Focus()
	}
	if len(parts) != 3 {
		return nil
	}
	for _, act := range actions {
		if act.ID != parts[2] {
			continue
		}
		m.closeModal()
		prepare := func() (*exec.Cmd, error) { return item.Command(act, p) }
		switch act.Kind {
		case "url":
			return func() tea.Msg {
				err := m.launcher.OpenURL(item.Expand(act.URL, p))
				return actionMsg{id: p.ID, notice: act.Name + " opened", err: err}
			}
		case "output":
			return m.runAddonOutput(p, item.Definition.Name+" / "+act.Name, prepare)
		case "app":
			return func() tea.Msg {
				cmd, err := prepare()
				if err == nil {
					err = tools.OpenApplication(cmd)
				}
				return actionMsg{id: p.ID, notice: act.Name + " opened", err: err}
			}
		default:
			mode := m.cfg.Settings.LaunchMode("cli")
			if act.Kind == "command" {
				mode = m.cfg.Settings.LaunchMode("run")
			}
			launch := func(mode string) tea.Cmd { return m.launchInteractiveMode(mode, p, prepare, act.Name, true) }
			if mode == "ask" {
				return m.askLaunch(act.Name, launch)
			}
			return launch(mode)
		}
	}
	return m.notify("Action unavailable", fmt.Errorf("the addon no longer matches this project"))
}
func (m *Model) runAddonOutput(p project.Project, title string, prepare func() (*exec.Cmd, error)) tea.Cmd {
	m.outputDone = nil
	m.mode = gitOutput
	m.outputPrepare = prepare
	m.gitProject = p
	m.gitTitle = title
	m.gitSteps = nil
	m.gitStep = 0
	m.gitLog = ""
	m.gitError = ""
	m.gitRunning = true
	m.gitJobStarted = false
	m.gitJobID = fmt.Sprintf("addon:%s:%d", p.ID, time.Now().UnixNano())
	m.resize()
	m.viewport.SetContent("Starting " + oneLine(title))
	m.viewport.GotoTop()
	job, manager := m.gitJobID, m.manager
	return func() tea.Msg {
		cmd, err := prepare()
		if err != nil {
			return gitRunMsg{job: job, err: err}
		}
		state, err := manager.StartCommand(job, title, cmd)
		return gitRunMsg{job: job, state: state, err: err}
	}
}
