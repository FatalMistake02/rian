package ui

import (
	"fmt"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rian/internal/config"
	"rian/internal/detect"
	"rian/internal/project"
	"rian/internal/tools"
)

type settingRow struct {
	id, label, section, projectID    string
	launch, page                     bool
	addonID, addonSetting, operation string
}

var shortcutSettings = []struct{ id, label string }{
	{"run", "Commands"}, {"files", "Files"},
	{"git", "Git actions"}, {"github", "GitHub"}, {"kilocode", "KiloCode"},
	{"lazygit", "lazygit"}, {"notes", "Notes"}, {"opencode", "OpenCode"}, {"terminal", "Terminal"},
	{"vscode", "VS Code"},
}

func (m *Model) startSettings() {
	m.mode, m.formError, m.settingsIndex, m.settingsPage = settings, "", 0, ""
	m.settingsSearching = false
	m.settingsSearch = input("Search")
	m.settingsOriginal = m.cfg.Settings.Clone()
	m.settingsDraft = m.cfg.Settings.Clone()
	m.settingsChanged = false
	m.buildSettingsRows()
	if m.projectFull {
		if p, ok := m.current(); ok {
			m.openSettingsPage("project settings/" + p.ID)
		}
	}
	m.resize()
}

func (m *Model) buildSettingsRows() {
	m.settingsRows = nil
	for _, name := range []string{"appearance", "behavior", "shortcuts", "project settings", "addons"} {
		m.settingsRows = append(m.settingsRows, settingRow{id: name, label: name, page: true})
	}
	m.settingsRows = append(m.settingsRows, settingRow{id: "accent", label: "Accent color", section: "appearance"}, settingRow{id: "startup", label: "Default opening page", section: "behavior"}, settingRow{id: "project-startup", label: "When launched from a project", section: "behavior"})
	m.settingsRows = append(m.settingsRows, settingRow{id: "graph-scope", label: "Git graph branches", section: "behavior"})
	for _, item := range []struct{ id, label string }{{"cli", "CLI tools open in"}, {"run", "Commands open in"}} {
		m.settingsRows = append(m.settingsRows, settingRow{id: item.id, label: item.label, launch: true, section: "behavior"})
	}
	for _, item := range shortcutSettings {
		m.settingsRows = append(m.settingsRows, settingRow{id: item.id, label: item.label, section: "shortcuts"})
	}
	for _, p := range m.cfg.Projects {
		page := "project settings/" + p.ID
		m.settingsRows = append(m.settingsRows, settingRow{id: page, label: p.Name, section: "project settings", page: true, projectID: p.ID})
		for _, item := range shortcutSettings {
			m.settingsRows = append(m.settingsRows, settingRow{id: item.id, label: item.label, section: page, projectID: p.ID})
		}
		for _, name := range detect.CommandNames(p.AllCommands()) {
			m.settingsRows = append(m.settingsRows, settingRow{id: config.CommandKey(p.ID, name), label: name, section: page, projectID: p.ID})
		}
	}
	m.settingsRows = append(m.settingsRows,
		settingRow{id: "behavior/auto add", label: "Auto add", section: "behavior", page: true},
		settingRow{id: "auto-add-enabled", label: "Enabled", section: "behavior/auto add"},
		settingRow{id: "auto-add-location", label: "Location", section: "behavior/auto add"},
		settingRow{id: "auto-add-hidden", label: "Include hidden folders", section: "behavior/auto add"})
	m.addonSettingsRows()
}

func (m *Model) settingsPath(page string) string {
	if strings.HasPrefix(page, "addons/") {
		if item, ok := m.settingsDraft.Addons[strings.TrimPrefix(page, "addons/")]; ok {
			return "addons / " + oneLine(item.Definition.Name) + " · " + item.Definition.Version
		}
	}
	if strings.HasPrefix(page, "project settings/") {
		id := strings.TrimPrefix(page, "project settings/")
		for _, p := range m.cfg.Projects {
			if p.ID == id {
				return "project settings / " + oneLine(p.Name)
			}
		}
	}
	return strings.ReplaceAll(page, "/", " / ")
}
func (m *Model) settingsMatches() []settingRow {
	query := m.settingsSearch.Value()
	rows := []settingRow{}
	for _, row := range m.settingsRows {
		if !m.settingAvailable(row) {
			continue
		}
		if query == "" {
			if row.section == m.settingsPage {
				rows = append(rows, row)
			}
			continue
		}
		if m.settingsPage != "" && row.section != m.settingsPage && !strings.HasPrefix(row.section, m.settingsPage+"/") {
			continue
		}
		text := row.label + " " + m.settingsPath(row.section) + " " + m.settingValue(row)
		if project.Score(query, text) >= 0 || strings.Contains(strings.ToLower(text), strings.ToLower(query)) {
			rows = append(rows, row)
		}
	}
	return rows
}
func (m *Model) openSettingsPage(page string) {
	m.settingsPage, m.settingsIndex = page, 0
	m.settingsSearch.Reset()
	m.settingsSearch.Blur()
	m.settingsSearching = false
	m.resize()
}
func (m *Model) settingsInput(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+s" || key == "esc" || key == "ctrl+c" {
		return m.settingsKey(key)
	}
	if m.settingsSearching {
		switch key {

		case "backspace":
			if m.settingsSearch.Value() == "" {
				m.settingsSearch.Blur()
				m.settingsSearching = false
				return nil
			}
		case "up", "down":
			return m.settingsKey(key)
		case "enter":
			rows := m.settingsMatches()
			if len(rows) == 0 {
				return nil
			}
			row := rows[min(m.settingsIndex, len(rows)-1)]
			if row.page {
				m.openSettingsPage(row.id)
			} else {
				m.openSettingsPage(row.section)
				for i, item := range m.settingsMatches() {
					if item.id == row.id && item.projectID == row.projectID {
						m.settingsIndex = i
						break
					}
				}
			}
			return nil
		}
		var cmd tea.Cmd
		m.settingsSearch, cmd = m.settingsSearch.Update(msg)
		m.settingsIndex = 0
		return cmd
	}
	return m.settingsKey(key)
}
func (m *Model) settingsKey(key string) tea.Cmd {
	rows := m.settingsMatches()
	switch key {
	case "/":
		m.settingsSearching = true
		return m.settingsSearch.Focus()
	case "esc":
		if m.settingsSearching {
			m.settingsSearch.Reset()
			m.settingsSearch.Blur()
			m.settingsSearching = false
			m.settingsIndex = 0
			return nil
		}
		if m.settingsPage != "" {
			if strings.Contains(m.settingsPage, "/") {
				m.openSettingsPage(m.settingsPage[:strings.LastIndex(m.settingsPage, "/")])
			} else {
				m.openSettingsPage("")
			}
			return nil
		}
		return m.keepSettings()
	case "ctrl+s":
		return m.keepSettings()
	case "ctrl+c":
		hash := m.selectedGraphHash()
		m.settingsDraft = m.settingsOriginal.Clone()
		m.settingsChanged = false
		m.cfg.Settings = m.settingsOriginal.Clone()
		m.restoreGraphSelection(hash)
		m.applyAppearance()
		m.closeModal()

	case "up", "j", "shift+tab":
		m.settingsIndex = max(0, m.settingsIndex-1)
	case "down", "k", "tab":
		m.settingsIndex = min(max(0, len(rows)-1), m.settingsIndex+1)
	case "backspace":
		if len(rows) > 0 {
			row := rows[min(m.settingsIndex, len(rows)-1)]
			if row.projectID != "" && !row.page {
				delete(m.settingsDraft.Overrides[row.projectID], row.id)
				m.applySettingsChange()
			}
		}
	case " ", "space", "enter", "left", "right":
		if len(rows) == 0 {
			return nil
		}
		row := rows[min(m.settingsIndex, len(rows)-1)]
		if row.page {
			m.openSettingsPage(row.id)
			return nil
		}
		if row.id == "auto-add-location" {
			m.mode, m.formError = autoAddLocation, ""
			m.query = input("Leave empty to select, or type a path")
			m.query.SetValue(m.settingsDraft.AutoAddLocation)
			m.resize()
			return m.query.Focus()
		}
		if row.id == "auto-add-enabled" || row.id == "auto-add-hidden" {
			if row.id == "auto-add-enabled" {
				m.settingsDraft.AutoAddEnabled = !m.settingsDraft.AutoAddEnabled
			} else {
				m.settingsDraft.AutoAddHidden = !m.settingsDraft.AutoAddHidden
			}
			m.applySettingsChange()
			return m.scanAutoAdd()
		}
		if row.operation != "" {
			return m.changeAddonSetting(row, key)
		}
		if row.id == "graph-scope" {
			if m.settingsDraft.GitGraphScope() == "current" {
				m.settingsDraft.GraphScope = "all"
			} else {
				m.settingsDraft.GraphScope = "current"
			}
			m.applySettingsChange()
			return nil
		}
		if row.id == "project-startup" {
			if m.settingsDraft.AutoOpenProject() {
				m.settingsDraft.ProjectOpening = "normal"
			} else {
				m.settingsDraft.ProjectOpening = "project"
			}
			m.applySettingsChange()
			return nil
		}
		if row.id == "startup" {
			pages := []string{"overview", "projects"}
			index := 0
			for i, page := range pages {
				if page == m.settingsDraft.StartPage() {
					index = i
				}
			}
			step := 1
			if key == "left" {
				step = len(pages) - 1
			}
			m.settingsDraft.DefaultPage = pages[(index+step)%len(pages)]
			m.applySettingsChange()
			return nil
		}
		if row.id == "accent" {
			index := 0
			for i, color := range config.AccentColors {
				if color.ID == m.settingsDraft.AccentColor().ID {
					index = i
				}
			}
			step := 1
			if key == "left" {
				step = len(config.AccentColors) - 1
			}
			m.settingsDraft.Accent = config.AccentColors[(index+step)%len(config.AccentColors)].ID
			m.applySettingsChange()
			return nil
		}
		if !row.launch {
			if row.projectID != "" {
				if m.settingsDraft.Overrides[row.projectID] == nil {
					m.settingsDraft.Overrides[row.projectID] = map[string]bool{}
				}
				m.settingsDraft.Overrides[row.projectID][row.id] = !m.draftShown(row)
			} else {
				m.settingsDraft.Hidden[row.id] = !m.settingsDraft.Hidden[row.id]
			}
			m.applySettingsChange()
			return nil
		}
		modes := append(tools.TerminalModes(), "ask")
		index := 0
		for i, mode := range modes {
			if mode == m.settingsDraft.LaunchMode(row.id) {
				index = i
			}
		}
		step := 1
		if key == "left" {
			step = len(modes) - 1
		}
		m.settingsDraft.Launch[row.id] = modes[(index+step)%len(modes)]
		m.applySettingsChange()
	}
	return nil
}
func (m *Model) applySettingsChange() {
	m.settingsChanged = true
	hash := m.selectedGraphHash()
	m.cfg.Settings = m.settingsDraft.Clone()
	m.restoreGraphSelection(hash)
	m.applyAppearance()
}
func (m *Model) keepSettings() tea.Cmd {
	if !m.settingsChanged {
		m.closeModal()
		return nil
	}
	draft := m.settingsDraft.Clone()
	original := m.settingsOriginal.Clone()
	id := ""
	if p, ok := m.current(); ok {
		id = p.ID
	}
	cmd := m.save(id, "Settings saved", func(cfg *config.Config) error {
		latest := cfg.Settings.Clone().Addons
		if reflect.DeepEqual(draft.Addons, original.Addons) {
			draft.Addons = latest
		} else if !reflect.DeepEqual(latest, original.Addons) {
			return fmt.Errorf("addons changed while settings were open; restart Rian and reopen Settings before saving")
		}
		cfg.Settings = draft
		return nil
	})
	m.closeModal()
	return cmd
}

func launchLabel(mode string) string {
	switch mode {
	case "ask":
		return "Ask each time"
	case "window":
		return "New window"
	default:
		return "Same terminal"
	}
}
func (m *Model) settingValue(row settingRow) string {
	if row.id == "auto-add-location" {
		if m.settingsDraft.AutoAddLocation == "" {
			return "Not set"
		}
		return oneLine(m.settingsDraft.AutoAddLocation)
	}
	if row.id == "auto-add-enabled" || row.id == "auto-add-hidden" {
		enabled := m.settingsDraft.AutoAddEnabled
		if row.id == "auto-add-hidden" {
			enabled = m.settingsDraft.AutoAddHidden
		}
		if enabled {
			return "Enabled"
		}
		return "Disabled"
	}
	if row.operation != "" {
		return m.addonSettingValue(row)
	}
	if row.page {
		if row.addonID != "" {
			if m.settingsDraft.Addons[row.addonID].Enabled {
				return "Enabled ›"
			}
			return "Disabled ›"
		}
		return "›"
	}
	if row.id == "graph-scope" {
		if m.settingsDraft.GitGraphScope() == "all" {
			return "All"
		}
		return "Current"
	}
	if row.id == "project-startup" {
		if m.settingsDraft.AutoOpenProject() {
			return "Project"
		}
		return "Default"
	}
	if row.id == "startup" {
		switch m.settingsDraft.StartPage() {
		case "projects":
			return "Projects"
		default:
			return "Overview"
		}
	}
	if row.id == "accent" {
		return m.settingsDraft.AccentColor().Name
	}
	if row.launch {
		return launchLabel(m.settingsDraft.LaunchMode(row.id))
	}
	shown := m.settingsDraft.Shown(row.id)
	if row.projectID != "" {
		shown = m.draftShown(row)
	}
	if shown {
		return "Show"
	}
	return "Hide"
}
func (m *Model) settingsHeader(width int) string {
	title := "SETTINGS"
	if m.settingsPage != "" {
		title += muted.Render(" / ") + m.settingsPath(m.settingsPage)
	}
	if m.settingsSearching || m.settingsSearch.Value() != "" {
		title = fit(title, max(8, width-12))
		search := m.settingsSearch
		search.SetWidth(max(1, width-lipgloss.Width(title)-4))
		title += "  " + highlight.Render("/ ") + search.View()
	}
	return fit(title, width)
}
func (m *Model) settingsFooter() string {
	middle := keyHint("/", "search")
	if strings.HasPrefix(m.settingsPage, "project settings/") {
		middle += "  " + keyHint("⌫", "reset")
	}
	selection := keyHint("←→ / Space", "change")
	rows := m.settingsMatches()
	if m.settingsPage == "" || (len(rows) > 0 && rows[min(m.settingsIndex, len(rows)-1)].page) {
		selection = keyHint("Space / → / Enter", "select")
	}
	if len(rows) > 0 && rows[min(m.settingsIndex, len(rows)-1)].id == "auto-add-location" {
		selection = keyHint("Enter / Space", "edit")
	}
	return keyHint("Esc", "back") + "  " + keyHint("↑↓", "choose") + "\n" + selection + "\n" + middle + "  " + keyHint("Ctrl+C", "cancel")
}
func (m *Model) settingsView(width int) string {
	rows := m.settingsMatches()
	if len(rows) == 0 {
		return muted.Render("No matching settings.")
	}
	count := max(1, min(20, m.height-10-len(strings.Split(m.settingsFooter(), "\n"))))
	stride := 1
	if m.settingsSearch.Value() != "" {
		stride = 2
		count = max(1, count/2)
	}
	start := max(0, m.settingsIndex-count+1)
	valueWidth := min(16, width/2)
	labelWidth := max(1, width-valueWidth-3)
	lines := []string{}
	for i := start; i < min(len(rows), start+count); i++ {
		row := rows[i]
		value := oneLine(m.settingValue(row))
		prefix := "  "
		if i == m.settingsIndex {
			prefix = "› "
		}
		valueStyle := highlight
		if row.id == "accent" {
			valueStyle = valueStyle.Foreground(lipgloss.Color(m.effectiveAccent(m.settingsDraft).Hex))
		}
		if row.projectID != "" && !row.page {
			if _, set := m.settingsDraft.Override(row.projectID, row.id); !set {
				valueStyle = muted
			}
		}
		text := pad(prefix+oneLine(row.label), labelWidth) + "   " + strings.Repeat(" ", max(0, valueWidth-lipgloss.Width(value))) + valueStyle.Render(fit(value, valueWidth))
		if i == m.settingsIndex {
			text = selectedStyle.Render(text)
		}
		lines = append(lines, text)
		if stride == 2 {
			path := row.section
			if row.page {
				path = row.id
			}
			lines = append(lines, muted.Render(fit("  "+m.settingsPath(path), width)))
		}
	}
	return strings.Join(lines, "\n")
}
func projectSettingShown(settings config.Settings, p project.Project, id string) bool {
	if value, set := settings.Override(p.ID, id); set {
		return value
	}
	return settings.Shown(id) && p.ToolEnabled(id)
}
func (m *Model) shownFor(p project.Project, id string) bool {
	return projectSettingShown(m.cfg.Settings, p, id)
}
func (m *Model) draftShown(row settingRow) bool {
	for _, p := range m.cfg.Projects {
		if p.ID == row.projectID {
			return projectSettingShown(m.settingsDraft, p, row.id)
		}
	}
	return m.settingsDraft.ShownFor(row.projectID, row.id)
}
func (m *Model) visibleCommands(p project.Project) map[string]string {
	commands := map[string]string{}
	if !m.shownFor(p, "run") {
		return commands
	}
	for name, cmd := range p.AllCommands() {
		if m.shownFor(p, config.CommandKey(p.ID, name)) {
			commands[name] = cmd
		}
	}
	return commands
}
func (m *Model) actionShown(action Action) bool {
	p, ok := m.current()
	if !ok {
		return m.cfg.Settings.Shown(action.ID)
	}
	id := action.ID
	if id == "commit" || id == "branch" || id == "switchBranch" || id == "newBranch" || id == "addRemote" || id == "remoteChoice" {
		id = "git"
	}
	if !m.shownFor(p, id) {
		return false
	}
	if action.ID == "run" {
		return m.shownFor(p, config.CommandKey(p.ID, action.Value))
	}
	return true
}

func (m *Model) askLaunch(label string, launch func(string) tea.Cmd) tea.Cmd {
	m.mode, m.launchTitle, m.pendingLaunch = launchChoice, label, launch
	m.launchModes, m.launchIndex = tools.TerminalModes(), 0
	for i, mode := range m.launchModes {
		if mode == "window" {
			m.launchIndex = i
		}
	}
	return nil
}
func (m *Model) launchChoiceKey(key string) tea.Cmd {
	switch key {
	case "up", "j":
		m.launchIndex = max(0, m.launchIndex-1)
	case "down", "k":
		m.launchIndex = min(len(m.launchModes)-1, m.launchIndex+1)
	case "enter":
		launch, mode := m.pendingLaunch, m.launchModes[m.launchIndex]
		m.closeModal()
		if launch != nil {
			return launch(mode)
		}
	}
	return nil
}
func (m *Model) launchChoiceView(width int) string {
	lines := []string{fit(oneLine(m.launchTitle), width), ""}
	for i, mode := range m.launchModes {
		prefix := "  "
		if i == m.launchIndex {
			prefix = "› "
		}
		line := prefix + launchLabel(mode)
		if i == m.launchIndex {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) shortcutCapable(id string, p project.Project) bool {
	switch id {
	case "vscode":
		return m.available["code"]
	case "opencode", "kilocode", "lazygit":
		return m.available[id]
	case "files":
		return m.available["files"]
	case "github":
		_, err := tools.GitHubURL(p, m.statuses[p.ID])
		return err == nil
	case "git":
		return m.gitMenuAvailable(p)
	case "run":
		return len(p.AllCommands()) > 0
	case "process":
		return m.manager.Snapshot(p.ID).Command != ""
	default:
		return true
	}
}
func (m *Model) settingAvailable(row settingRow) bool {
	if row.operation != "" {
		return true
	}
	if strings.HasPrefix(row.id, "addon/") {
		id := strings.TrimPrefix(row.id, "addon/")
		item, ok := m.settingsDraft.Addons[id]
		if !ok {
			return false
		}
		for _, p := range m.cfg.Projects {
			if row.projectID != "" && row.projectID != p.ID {
				continue
			}
			if len(item.Actions(p)) > 0 {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(row.id, "auto-add-") || row.page || row.launch || row.id == "accent" || row.id == "startup" || row.id == "project-startup" || row.id == "graph-scope" || strings.HasPrefix(row.id, "command/") {
		return true
	}
	if row.projectID != "" {
		for _, p := range m.cfg.Projects {
			if p.ID == row.projectID {
				return m.shortcutCapable(row.id, p)
			}
		}
		return false
	}
	if row.id == "github" || row.id == "git" || row.id == "run" || row.id == "process" {
		for _, p := range m.cfg.Projects {
			if m.shortcutCapable(row.id, p) {
				return true
			}
		}
		return false
	}
	return m.shortcutCapable(row.id, project.Project{})
}
