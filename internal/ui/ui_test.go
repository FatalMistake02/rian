package ui

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"rian/internal/config"
	"rian/internal/detect"
	"rian/internal/git"
	"rian/internal/process"
	"rian/internal/project"
)

func fixture(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	m := New(config.New(filepath.Join(dir, "config.json")), process.New(), log.New(io.Discard, "", 0))
	m.overview = false
	m.width, m.height = 120, 36
	m.resize()
	p := project.New("Cardly", dir)
	p.Favorite = true
	p.Stack = []string{"Node.js", "pnpm", "Next.js"}
	p.AutoCommands = map[string]string{"dev": "pnpm run dev", "build": "pnpm run build", "test": "pnpm run test"}
	p.LastUsed = time.Now().Add(-2 * time.Hour)
	m.cfg = config.Config{Version: 1, Projects: []project.Project{p, project.New("Drift", filepath.Join(dir, "drift")), project.New("Rin", filepath.Join(dir, "rin"))}}
	m.statuses[p.ID] = git.Status{Repository: true, Branch: "main", Modified: 3, Ahead: 1, GitHub: "FatalMistake02/Cardly", Commit: "Fix scanner cache", CommitTime: time.Now().Add(-2 * time.Hour)}
	m.available = map[string]bool{"code": true, "opencode": true, "kilocode": true}
	return m
}

func press(code rune, text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code, Text: text} }

func TestTextInputTakesPriority(t *testing.T) {
	m := fixture(t)
	m.key(press('/', "/"))
	for _, c := range "car" {
		m.key(press(c, string(c)))
	}
	if !m.searching || m.search.Value() != "car" || m.mode != home || len(m.projects()) != 1 {
		t.Fatalf("searching=%v query=%q mode=%d matches=%d", m.searching, m.search.Value(), m.mode, len(m.projects()))
	}
	m.key(press('q', "q"))
	if m.search.Value() != "carq" {
		t.Fatal("q must type while searching")
	}
	m.key(press(tea.KeyEscape, ""))
	if m.searching || m.search.Value() != "" {
		t.Fatal("Escape did not exit search")
	}
	m.openMenu(palette)
	m.key(press('k', "k"))
	if m.query.Value() != "k" || m.mode != palette {
		t.Fatal("palette input triggered KiloCode")
	}
	m.closeModal()
	m.selected = 1
	m.key(press('j', "j"))
	if m.selected != 0 {
		t.Fatal("j should navigate up")
	}
}

func TestAddConfirmationAndPersistence(t *testing.T) {
	m := fixture(t)
	m.cfg.Projects = nil
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test"), 0600)
	m.startAdd()
	m.query.SetValue(dir)
	inspect := m.key(press(tea.KeyEnter, ""))
	if inspect == nil {
		t.Fatal("inspection missing")
	}
	m.Update(inspect())
	if m.mode != addConfirm || len(m.cfg.Projects) != 0 {
		t.Fatal("add must preview before saving")
	}
	save := m.key(press(tea.KeyEnter, ""))
	m.Update(save())
	cfg, err := m.store.Load()
	if err != nil || len(cfg.Projects) != 1 || m.mode != home || cfg.Projects[0].AutoCommands["test"] != "go test ./..." {
		t.Fatalf("add failed: %+v %v", cfg, err)
	}
	m.execute(Action{ID: "remove"})
	if m.mode != removeConfirm {
		t.Fatal("removal needs confirmation")
	}
	m.key(press(tea.KeyEscape, ""))
	cfg, _ = m.store.Load()
	if len(cfg.Projects) != 1 {
		t.Fatal("cancelled removal changed config")
	}
}

func TestStaleSaveDoesNotRevertNewerState(t *testing.T) {
	m := fixture(t)
	newer := m.cfg
	newer.Revision = 10
	m.Update(savedMsg{cfg: newer})
	m.Update(savedMsg{cfg: config.Config{Version: 1, Revision: 9}})
	if m.cfg.Revision != 10 || len(m.cfg.Projects) != 3 {
		t.Fatal("out-of-order async save reverted the project list")
	}
}

func TestRecentUseWriteDoesNotUnblockPendingSave(t *testing.T) {
	m := fixture(t)
	m.busy = true
	m.Update(savedMsg{cfg: m.cfg})
	if !m.busy {
		t.Fatal("background recent-use write unblocked an unfinished edit")
	}
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {100, 30}, {80, 24}, {52, 16}, {40, 12}} {
		for _, which := range []mode{home, palette, commands, add, addConfirm, edit, prompt, removeConfirm, quitConfirm, processView, settings, launchChoice} {
			m := fixture(t)
			m.width, m.height = size[0], size[1]
			p, _ := m.current()
			m.mode = which
			switch which {
			case launchChoice:
				m.askLaunch("OpenCode", func(string) tea.Cmd { return nil })
			case settings:
				m.startSettings()
			case palette, commands:
				m.openMenu(which)
			case edit:
				m.startEdit(p)
			case add:
				m.startAdd()
			case addConfirm:
				m.pending = detect.Result{Project: p, Git: m.statuses[p.ID], Available: m.available}
				m.viewport.SetContent(m.detectionSummary(m.pending))
			case removeConfirm:
				m.editProject = p
			case processView:
				m.processState.Command = "pnpm run dev"
				m.viewport.SetContent(strings.Repeat("Recent output\n", 20))
			}
			m.resize()
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Errorf("mode %d at %v: actual %d × %d", which, size, lipgloss.Width(view), lipgloss.Height(view))
			}
			if size[0] >= 52 && which == home && !strings.Contains(view, "╰") {
				t.Error("bottom panel borders were clipped")
			}
			if size[0] >= 52 && which != home && !strings.Contains(plain(view), "Esc") && which != edit {
				t.Errorf("mode %d at %v: modal footer is hidden", which, size)
			}
			if dir := os.Getenv("RIAN_TEST_RENDER_DIR"); dir != "" && size == [2]int{120, 36} && (which == home || which == palette || which == edit || which == processView) {
				os.MkdirAll(dir, 0700)
				os.WriteFile(filepath.Join(dir, map[mode]string{home: "home", palette: "palette", edit: "edit", processView: "process"}[which]+".ansi"), []byte(view), 0600)
			}
		}
	}
}

func TestExternalOutputCannotControlTerminal(t *testing.T) {
	output := plain("\x1b[31mred\x1b[0m\x1b]0;hijacked\x07\x00\r\nline")
	if strings.ContainsAny(output, "\x1b\x00\x07\r") || !strings.Contains(output, "red") {
		t.Fatalf("unsafe terminal output %q", output)
	}
}

func TestWelcomeLayoutAndProjectTransition(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {80, 24}, {52, 16}} {
		m := fixture(t)
		projects := m.cfg.Projects
		m.cfg.Projects = nil
		m.width, m.height = size[0], size[1]
		view := plain(m.homeView())
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) != m.height {
			t.Fatalf("welcome at %v: actual %d × %d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if strings.Count(view, "╭") != 1 || strings.Count(view, "╰") != 1 || !strings.Contains(view, "add your first project") || !strings.Contains(view, "█") {
			t.Fatalf("welcome box or title missing at %v: %s", size, view)
		}
		lines := strings.Split(view, "\n")
		if !strings.HasPrefix(lines[0], "   RIAN") || !strings.HasPrefix(lines[len(lines)-1], "   Esc") || !strings.HasPrefix(lines[1], "  ╭") || !strings.HasPrefix(lines[len(lines)-2], "  ╰") {
			t.Fatalf("unequal header/footer spacing at %v", size)
		}
		if strings.Contains(view, "your project desk") || strings.Contains(view, "PROJECTS") {
			t.Fatal("welcome still shows the old dashboard")
		}
		m.cfg.Projects = projects[:1]
		view = plain(m.homeView())
		if strings.Count(view, "╭") != 2 || !strings.Contains(view, "PROJECTS") || !strings.Contains(view, "Cardly") {
			t.Fatal("first saved project did not restore the dashboard")
		}
		if dir := os.Getenv("RIAN_TEST_RENDER_DIR"); dir != "" && size[0] == 120 {
			m.cfg.Projects = nil
			os.MkdirAll(dir, 0700)
			os.WriteFile(filepath.Join(dir, "welcome.ansi"), []byte(m.homeView()), 0600)
		}
	}
}

func TestFolderPickerSelectionAndCancellation(t *testing.T) {
	m := fixture(t)
	m.startAdd()
	if strings.Contains(plain(m.modalView()), "Rian will inspect") {
		t.Fatal("old add-project instruction is still visible")
	}
	m.busy = true
	m.Update(pickedFolderMsg{})
	if m.busy || m.mode != add || m.query.Value() != "" {
		t.Fatal("canceling the picker did not return to the add prompt")
	}
	dir := t.TempDir()
	_, inspect := m.Update(pickedFolderMsg{path: dir})
	if inspect == nil || m.query.Value() != dir || !m.busy {
		t.Fatal("selected directory was not sent for inspection")
	}
	m.Update(inspect())
	if m.mode != addConfirm {
		t.Fatal("picker selection skipped the project preview")
	}
	m.closeModal()
	_, cmd := m.Update(pickedFolderMsg{path: dir})
	if cmd != nil || m.mode != home {
		t.Fatal("late picker result reopened a dismissed prompt")
	}
}

func TestTypingAfterPickerCloses(t *testing.T) {
	for _, pickerErr := range []error{nil, fmt.Errorf("picker failed")} {
		m := fixture(t)
		m.startAdd()
		m.query.Blur()
		m.busy = true
		m.Update(pickedFolderMsg{err: pickerErr})
		m.key(press('x', "x"))
		if m.busy || m.mode != add || m.query.Value() != "x" {
			t.Fatalf("typing did not recover after picker closed: err=%v", pickerErr)
		}
		if pickerErr == nil && m.formError != "" {
			t.Fatal("canceling picker displayed an error")
		}
	}
}

func TestProjectHintsOnlyShowAvailableActions(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	m.available = map[string]bool{"opencode": true}
	p.Tools = map[string]bool{"terminal": false}
	p.AutoCommands, p.Commands = nil, nil
	m.statuses[p.ID] = git.Status{}
	hints := plain(strings.Join(m.projectHints(p, 60), "\n"))
	for _, hidden := range []string{"VS Code", "KiloCode", "Terminal", "GitHub", "r Run", "p Output", "G Git"} {
		if strings.Contains(hints, hidden) {
			t.Errorf("unavailable hint %q shown: %s", hidden, hints)
		}
	}
	if !strings.Contains(hints, "o OpenCode") || !strings.Contains(hints, "r Commands") {
		t.Fatal("available tool or command creation was hidden")
	}
	m.statuses[p.ID] = git.Status{Repository: true}
	if strings.Contains(plain(strings.Join(m.projectHints(p, 60), "\n")), "g GitHub") {
		t.Fatal("Git repository without a GitHub remote showed GitHub shortcut")
	}
	p.GitHub = "FatalMistake02/rian"
	p.Commands = map[string]string{"test": "go test ./..."}
	m.statuses[p.ID] = git.Status{Repository: true}
	hints = plain(strings.Join(m.projectHints(p, 20), "\n"))
	for _, shown := range []string{"g GitHub", "r Commands", "G Git"} {
		if !strings.Contains(hints, shown) {
			t.Errorf("available hint %q hidden", shown)
		}
	}
	if lipgloss.Width(hints) > 20 {
		t.Fatal("hints overflow the panel")
	}
}

func TestRemovedToolDoesNotAppearFromLegacySettings(t *testing.T) {
	m := fixture(t)
	m.cfg.Projects[0].Tools = map[string]bool{"chatgpt": true}
	m.cfg.Settings.Hidden = map[string]bool{"chatgpt": false}
	p, _ := m.current()
	if hints := plain(strings.Join(m.projectHints(p, 60), "\n")); strings.Contains(hints, "ChatGPT") {
		t.Fatal("removed tool remained in project hints")
	}
	m.openMenu(palette)
	m.query.SetValue("ChatGPT")
	if len(m.filteredActions()) != 0 {
		t.Fatal("removed tool remained in action palette")
	}
	m.startSettings()
	m.settingsSearch.SetValue("ChatGPT")
	if len(m.settingsMatches()) != 0 {
		t.Fatal("removed tool remained in shortcut settings")
	}
	m.startEdit(p)
	for _, field := range m.fields {
		if field.label == "chatgpt" {
			t.Fatal("removed tool remained in project edit form")
		}
	}
}

func TestSettingsVisibilityAndLaunchChoices(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	m.startSettings()
	launchRows := 0
	for _, row := range m.settingsRows {
		if row.launch {
			launchRows++
		}
	}
	if launchRows != 2 {
		t.Fatal("expected only CLI and command launch settings")
	}
	m.settingsKey("space")
	if !m.cfg.Settings.Shown("vscode") {
		t.Fatal("unsaved settings changed live config")
	}
	m.cfg.Settings.Hidden = map[string]bool{"opencode": true, config.CommandKey(p.ID, "dev"): true}
	m.openMenu(palette)
	for _, action := range m.filteredActions() {
		if action.ID == "opencode" || action.ID == "run" && action.Value == "dev" {
			t.Fatal("hidden action shown in palette")
		}
	}
	if _, ok := m.visibleCommands(p)["build"]; !ok {
		t.Fatal("unhidden npm command disappeared")
	}
	m.cfg.Settings.Launch = map[string]string{"cli": "ask", "run": "ask"}
	prepared := false
	m.launchInteractive("opencode", p, func() (*exec.Cmd, error) { prepared = true; return exec.Command("opencode"), nil }, "OpenCode", false)
	if m.mode != launchChoice || prepared {
		t.Fatal("Ask launched before user choice")
	}
	m.launchModes = []string{"same", "window"}
	m.launchIndex = 0
	cmd := m.launchChoiceKey("enter")
	if cmd == nil || m.mode != home {
		t.Fatal("choice did not continue launch")
	}
	if _, ok := cmd().(preparedExecMsg); !ok || !prepared {
		t.Fatal("same terminal did not use interactive launch")
	}
	m.runCommand(p, "dev", "npm run dev")
	if m.mode != launchChoice {
		t.Fatal("npm command did not ask where to launch")
	}
	m.modalKey(press(tea.KeyEscape, ""))
	if m.mode != home || m.pendingLaunch != nil {
		t.Fatal("cancel retained pending launch")
	}
}

func TestNotificationsAnchorBottomRight(t *testing.T) {
	for _, kind := range []string{"toast", "working", "validation"} {
		m := fixture(t)
		text := "Project added"
		switch kind {
		case "toast":
			m.toast = text
		case "working":
			m.busy = true
			text = "Working"
		case "validation":
			m.startAdd()
			m.formError = "Directory unavailable"
			text = m.formError
		}
		view := plain(m.View().Content)
		lines := strings.Split(view, "\n")
		if !strings.Contains(lines[len(lines)-1], text) || strings.Count(view, text) != 1 {
			t.Fatalf("%s is not exclusively on the bottom row", kind)
		}
		if lipgloss.Width(lines[len(lines)-1]) != m.width-2 {
			t.Fatalf("%s is not right aligned: width=%d", kind, lipgloss.Width(lines[len(lines)-1]))
		}
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatal("notification overflows terminal")
		}
	}
}

func TestProjectSearchSharesHeader(t *testing.T) {
	m := fixture(t)
	if heading := strings.TrimSpace(strings.Split(plain(m.listView(40, 20)), "\n")[0]); heading != "PROJECTS" {
		t.Fatalf("header retains count: %q", heading)
	}
	m.key(press('/', "/"))
	for _, c := range "car" {
		m.key(press(c, string(c)))
	}
	lines := strings.Split(plain(m.listView(40, 20)), "\n")
	if !strings.Contains(lines[0], "PROJECTS  / car") || strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("search is not inline: %q", lines[:2])
	}
	m.key(press(tea.KeyEscape, ""))
	if strings.TrimSpace(strings.Split(plain(m.listView(40, 20)), "\n")[0]) != "PROJECTS" {
		t.Fatal("search did not clear from header")
	}
}

func TestBackspaceClosesEmptySearch(t *testing.T) {
	m := fixture(t)
	m.key(press('/', "/"))
	m.key(press('x', "x"))
	m.key(press(tea.KeyBackspace, ""))
	if !m.searching || m.search.Value() != "" {
		t.Fatal("backspace should delete the last character without closing search")
	}
	m.key(press(tea.KeyBackspace, ""))
	if m.searching || m.search.Focused() {
		t.Fatal("backspace on empty search did not close it")
	}
}

func TestJMovesUpAndKMovesDown(t *testing.T) {
	m := fixture(t)
	m.selected = 1
	m.key(press('j', "j"))
	if m.selected != 0 {
		t.Fatal("j did not move up")
	}
	m.key(press('k', "k"))
	if m.selected != 1 {
		t.Fatal("k did not move down")
	}
	m.startSettings()
	m.settingsKey("k")
	if m.settingsIndex != 1 {
		t.Fatal("k did not move down in settings")
	}
	m.settingsKey("j")
	if m.settingsIndex != 0 {
		t.Fatal("j did not move up in settings")
	}
	m.mode = processView
	m.viewport.SetContent(strings.Repeat("output\n", 30))
	m.viewport.SetYOffset(5)
	m.modalKey(press('j', "j"))
	if m.viewport.YOffset() != 4 {
		t.Fatal("j did not scroll up")
	}
	m.modalKey(press('k', "k"))
	if m.viewport.YOffset() != 5 {
		t.Fatal("k did not scroll down")
	}
}

func TestSettingsValuesAlign(t *testing.T) {
	m := fixture(t)
	m.startSettings()
	lines := strings.Split(plain(m.settingsView(60)), "\n")
	if strings.Contains(strings.Join(lines, "\n"), "VALUE") || strings.Contains(strings.Join(lines, "\n"), "SETTING") {
		t.Fatal("settings should not have column headings")
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if lipgloss.Width(line) != 60 {
			found := false
			for _, row := range m.settingsRows {
				if line == row.section {
					found = true
				}
			}
			if !found {
				t.Fatalf("unexpected settings layout: %q", line)
			}
			continue
		}
		if strings.Contains(line, "│") || strings.HasSuffix(line, " ") {
			t.Fatalf("value is not right aligned: %q", line)
		}
	}

}

func TestAppearanceSettingsSaveAndSections(t *testing.T) {
	m := fixture(t)
	defer func() { m.cfg.Settings.Accent = ""; m.applyAppearance() }()
	m.startSettings()
	if m.settingsDraft.AccentColor().Name != "fatal pink" {
		t.Fatal("default accent name changed")
	}
	sections := map[string]bool{}
	for _, row := range m.settingsRows {
		sections[row.section] = true
	}
	for _, name := range []string{"appearance", "behavior", "shortcuts", "project settings"} {
		if !sections[name] {
			t.Errorf("missing settings section %q", name)
		}
	}
	m.openSettingsPage("appearance")
	m.settingsKey("right")
	if m.settingsDraft.AccentColor().Name != "glacial blue" || m.cfg.Settings.AccentColor().Name != "glacial blue" {
		t.Fatal("accent did not apply immediately")
	}
	cfg := m.cfg
	cfg.Settings = m.settingsDraft.Clone()
	m.Update(savedMsg{cfg: cfg, notice: "Settings saved"})
	if accent != lipgloss.Color("#8ccfe8") || m.search.Styles().Cursor.Color != accent {
		t.Fatal("saved accent did not update interface and cursor")
	}
	m.startSettings()
	m.openSettingsPage("appearance")
	m.settingsKey("left")
	m.modalKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.cfg.Settings.AccentColor().Name != "glacial blue" {
		t.Fatal("cancel changed saved accent")
	}
}

func TestSettingsPagesAndContextualSearch(t *testing.T) {
	m := fixture(t)
	m.startSettings()
	roots := m.settingsMatches()
	if len(roots) != 5 {
		t.Fatalf("unexpected settings pages: %+v", roots)
	}
	for _, row := range roots {
		if row.label == "commands" {
			t.Fatal("standalone commands page remains")
		}
	}
	m.settingsKey("enter")
	if m.settingsPage != "appearance" || !strings.Contains(plain(m.settingsHeader(60)), "SETTINGS / appearance") {
		t.Fatal("page breadcrumb missing")
	}
	m.settingsInput(press('/', "/"))
	for _, c := range "color" {
		m.settingsInput(press(c, string(c)))
	}
	if len(m.settingsMatches()) != 1 || m.settingsMatches()[0].id != "accent" {
		t.Fatal("appearance search did not find accent")
	}
	if !strings.Contains(plain(m.settingsHeader(60)), "/ color") {
		t.Fatal("search is not inline with the page header")
	}
	m.settingsInput(press(tea.KeyEscape, ""))
	if m.mode != settings || m.settingsSearching {
		t.Fatal("escape did not leave settings search")
	}
	m.settingsKey("esc")
	if m.settingsPage != "" {
		t.Fatal("escape did not go back to settings pages")
	}
	m.settingsKey("esc")
	if m.mode != home {
		t.Fatal("escape from settings root did not close")
	}
	m.startSettings()

	m.settingsInput(press('/', "/"))
	for _, c := range "opencode" {
		m.settingsInput(press(c, string(c)))
	}
	rows := m.settingsMatches()
	if len(rows) != 4 {
		t.Fatalf("search did not reach nested project overrides: %+v", rows)
	}
	view := plain(m.settingsView(70))
	if !strings.Contains(view, "shortcuts") || !strings.Contains(view, "project settings / Cardly") {
		t.Fatal("search results lack section context")
	}
	m.settingsInput(press(tea.KeyEnter, ""))
	if m.settingsPage != "shortcuts" || m.settingsSearching {
		t.Fatal("search selection did not open its page")
	}
	m.settingsInput(press('/', "/"))
	m.settingsInput(press('k', "k"))
	if m.settingsSearch.Value() != "k" {
		t.Fatal("navigation key did not type in settings search")
	}
	m.settingsInput(press(tea.KeyBackspace, ""))
	m.settingsInput(press(tea.KeyBackspace, ""))
	if m.settingsSearching {
		t.Fatal("backspace did not close empty settings search")
	}
}

func TestPerProjectShortcutOverrides(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	m.cfg.Settings.Hidden = map[string]bool{"opencode": true}
	m.startSettings()
	m.openSettingsPage("project settings/" + p.ID)
	for i, row := range m.settingsMatches() {
		if row.id == "opencode" {
			m.settingsIndex = i
		}
	}
	if m.settingValue(m.settingsMatches()[m.settingsIndex]) != "Hide" {
		t.Fatal("project did not inherit global hide")
	}
	m.settingsKey("right")
	if m.settingValue(m.settingsMatches()[m.settingsIndex]) != "Show" {
		t.Fatal("project override did not toggle to Show")
	}
	m.cfg.Settings = m.settingsDraft.Clone()
	if !strings.Contains(plain(strings.Join(m.projectHints(p, 60), "\n")), "o OpenCode") {
		t.Fatal("project Show override did not restore available shortcut")
	}
	if m.shownFor(m.cfg.Projects[1], "opencode") {
		t.Fatal("override changed another project")
	}
	m.settingsKey("backspace")
	if _, set := m.settingsDraft.Override(p.ID, "opencode"); set {
		t.Fatal("reset retained override")
	}
	if m.settingValue(m.settingsMatches()[m.settingsIndex]) != "Hide" {
		t.Fatal("reset did not restore global visibility")
	}
	m.cfg.Settings.Overrides[p.ID][config.CommandKey(p.ID, "dev")] = false
	if _, set := m.visibleCommands(p)["dev"]; set {
		t.Fatal("project command override was ignored")
	}
	if _, set := m.visibleCommands(p)["build"]; !set {
		t.Fatal("override hid an unrelated command")
	}
}

func TestSettingsApplyImmediatelyAndKeepOrCancel(t *testing.T) {
	m := fixture(t)
	defer func() { m.cfg.Settings = config.Settings{}; m.applyAppearance() }()
	if _, err := m.store.Update(func(cfg *config.Config) error { cfg.Projects = m.cfg.Projects; return nil }); err != nil {
		t.Fatal(err)
	}
	m.startSettings()
	m.openSettingsPage("shortcuts")
	for i, row := range m.settingsMatches() {
		if row.id == "opencode" {
			m.settingsIndex = i
			break
		}
	}
	m.settingsKey("right")
	if m.cfg.Settings.Shown("opencode") {
		t.Fatal("shortcut change did not apply immediately")
	}
	m.openSettingsPage("appearance")
	m.settingsKey("right")
	if accent != lipgloss.Color("#8ccfe8") {
		t.Fatal("accent did not preview immediately")
	}
	old := m.cfg
	old.Settings = config.Settings{}
	old.Revision++
	m.Update(savedMsg{cfg: old})
	if m.cfg.Settings.Shown("opencode") || m.cfg.Settings.AccentColor().Name != "glacial blue" {
		t.Fatal("background config update overwrote live settings")
	}
	m.settingsInput(press(tea.KeyEscape, ""))
	if m.settingsPage != "" {
		t.Fatal("Esc did not return to settings root")
	}
	cmd := m.settingsInput(press(tea.KeyEscape, ""))
	if m.mode != home || cmd == nil {
		t.Fatal("Esc did not close and keep changed settings")
	}
	m.Update(cmd())
	saved, err := m.store.Load()
	if err != nil || saved.Settings.Shown("opencode") || saved.Settings.AccentColor().Name != "glacial blue" {
		t.Fatal("Esc did not persist settings")
	}
	m.startSettings()
	m.openSettingsPage("appearance")
	m.settingsKey("right")
	m.openSettingsPage("shortcuts")
	for i, row := range m.settingsMatches() {
		if row.id == "opencode" {
			m.settingsIndex = i
			break
		}
	}
	m.settingsKey("right")
	m.settingsInput(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.mode != home || m.cfg.Settings.Shown("opencode") || accent != lipgloss.Color("#8ccfe8") {
		t.Fatal("Ctrl+C did not restore opening settings")
	}
	saved, _ = m.store.Load()
	if saved.Settings.Shown("opencode") || saved.Settings.AccentColor().Name != "glacial blue" {
		t.Fatal("cancel changed persisted settings")
	}
}

func TestShortcutSettingsOnlyListAvailableActions(t *testing.T) {
	m := fixture(t)
	m.available = map[string]bool{"opencode": true, "files": true}
	m.cfg.Settings.Hidden = map[string]bool{"opencode": true}
	m.startSettings()
	m.openSettingsPage("shortcuts")
	found := map[string]bool{}
	for _, row := range m.settingsMatches() {
		found[row.id] = true
	}
	if found["kilocode"] || found["vscode"] || found["lazygit"] {
		t.Fatal("uninstalled tools appear in shortcut settings")
	}
	if !found["opencode"] || !found["files"] {
		t.Fatal("hidden but available shortcut cannot be configured")
	}
	p := m.cfg.Projects[1]
	m.openSettingsPage("project settings/" + p.ID)
	for _, row := range m.settingsMatches() {
		if row.id == "github" || row.id == "git" || row.id == "kilocode" {
			t.Fatalf("unavailable project shortcut setting %q shown", row.id)
		}
	}
}

func TestEnterOpensFullProjectView(t *testing.T) {
	m := fixture(t)
	m.key(press(tea.KeyEnter, ""))
	if !m.projectFull {
		t.Fatal("Enter did not open project")
	}
	view := plain(m.View().Content)
	if strings.Count(view, "╭") != 3 || !strings.Contains(view, "GIT GRAPH") || strings.Contains(view, "PROJECTS") || !strings.Contains(view, "Cardly") || !strings.Contains(view, "pnpm run dev") {
		t.Fatal("project view is not a full project page")
	}
	if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
		t.Fatal("full project view overflows terminal")
	}
	m.startSettings()
	p, _ := m.current()
	if m.settingsPage != "project settings/"+p.ID {
		t.Fatal("project view did not open its own settings")
	}
	m.settingsKey("esc")
	m.settingsKey("esc")
	m.settingsKey("esc")
	if !m.projectFull || m.mode != home {
		t.Fatal("closing settings did not return to project view")
	}
	m.key(press(tea.KeyEscape, ""))
	if m.projectFull {
		t.Fatal("Esc did not return to the project list")
	}
}

func TestCurrentProjectOpeningOption(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		m := fixture(t)
		root := t.TempDir()
		nested := filepath.Join(root, "nested")
		if err := os.Mkdir(nested, 0700); err != nil {
			t.Fatal(err)
		}
		p := project.New("Current project", root)
		cfg := m.cfg
		cfg.Projects = append(cfg.Projects, p)
		if enabled {
			cfg.Settings.ProjectOpening = "project"
		} else {
			cfg.Settings.ProjectOpening = "normal"
		}
		m.startupDir = nested
		m.Update(readyMsg{cfg: cfg, available: m.available})
		if m.projectFull != enabled {
			t.Fatalf("startup project view enabled=%v full=%v", enabled, m.projectFull)
		}
		if enabled {
			current, _ := m.current()
			if current.ID != p.ID {
				t.Fatal("startup selected wrong project")
			}
		}
	}
	m := fixture(t)
	m.startupDir = t.TempDir()
	m.Update(readyMsg{cfg: m.cfg, available: m.available})
	if m.projectFull {
		t.Fatal("unrelated working directory opened a project view")
	}
}

func TestRandomAccentIsStableAndCancelRestoresOpeningColor(t *testing.T) {
	m := fixture(t)
	defer func() { m.cfg.Settings = config.Settings{}; m.applyAppearance() }()
	m.cfg.Settings.Accent = "random"
	m.randomAccent = config.AccentColors[2]
	m.applyAppearance()
	original := accent
	m.applyAppearance()
	if accent != original || original != lipgloss.Color(config.AccentColors[2].Hex) {
		t.Fatal("random color changed during the session")
	}
	m.startSettings()
	m.openSettingsPage("appearance")
	m.settingsKey("right")
	if accent == original {
		t.Fatal("accent edit did not apply")
	}
	m.openSettingsPage("behavior")
	m.settingsIndex = 0
	m.settingsKey("right")
	m.settingsKey("ctrl+c")
	if m.mode != home || m.cfg.Settings.Accent != "random" || accent != original || m.cfg.Settings.StartPage() != "overview" {
		t.Fatal("Ctrl+C did not restore the complete opening snapshot")
	}
}

func TestSettingsCancelThroughKeyboardRestoresOpeningSnapshot(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		m := fixture(t)
		m.cfg.Settings.Accent = "glacial-blue"
		m.cfg.Settings.Overrides = map[string]map[string]bool{}
		p, _ := m.current()
		m.cfg.Settings.Overrides[p.ID] = map[string]bool{"opencode": false}
		m.key(press(tea.KeyEnter, ""))
		m.key(press('s', "s"))
		for i, row := range m.settingsMatches() {
			if row.id == "opencode" {
				m.settingsIndex = i
				break
			}
		}
		m.settingsKey("right")
		m.openSettingsPage("appearance")
		m.settingsKey("right")
		m.settingsKey("/")
		m.key(key)
		if m.mode != home || !m.projectFull || m.cfg.Settings.Accent != "glacial-blue" || m.cfg.Settings.ShownFor(p.ID, "opencode") {
			t.Fatalf("cancel failed through keyboard dispatch for %v", key)
		}
		m.startSettings()
		if m.settingsDraft.ShownFor(p.ID, "opencode") || m.settingsDraft.Accent != "glacial-blue" {
			t.Fatal("reopening settings recovered canceled changes")
		}
	}
}

func TestProjectAndSettingsControls(t *testing.T) {
	m := fixture(t)
	m.available["files"] = true
	p, _ := m.current()
	view := plain(m.homeView())
	if !strings.Contains(view, "Enter open project") || !strings.Contains(view, "p unpin") {
		t.Fatal("project controls are missing")
	}
	if !strings.Contains(plain(strings.Join(m.projectHints(p, 60), "\n")), "f Files") {
		t.Fatal("Files shortcut is not f")
	}
	for _, action := range m.actions(p) {
		if strings.Contains(strings.ToLower(action.Label), "favorite") {
			t.Fatal("old favorite label remains")
		}
	}
	m.startSettings()
	footer := plain(m.settingsFooter())
	if !strings.Contains(footer, "Space / → / Enter select") || strings.Contains(footer, "change") {
		t.Fatal("settings pages do not advertise selection")
	}
	m.openSettingsPage("behavior")
	if !strings.Contains(plain(m.settingsView(70)), "Default opening page") {
		t.Fatal("startup label is unclear")
	}
	m.settingsKey("ctrl+c")
	m.key(press('/', "/"))
	m.key(press('c', "c"))
	m.key(press(tea.KeyEnter, ""))
	if !m.projectFull {
		t.Fatal("Enter from search did not open the project")
	}
	if strings.Count(plain(m.homeView()), "Ctrl+K") != 1 {
		t.Fatal("duplicate project action hint")
	}
}

func TestGitBranchAndCommitWorkflows(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", p.Path}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Rian Test")
	run("config", "user.email", "rian@example.test")
	run("config", "commit.gpgsign", "false")
	file := filepath.Join(p.Path, "file.txt")
	if err := os.WriteFile(file, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "--all")
	run("commit", "-m", "Initial commit")
	run("branch", "existing")
	cmd := m.execute(Action{ID: "branch"})
	m.Update(cmd())
	if m.mode != branchMenu {
		t.Fatal("branch picker did not open")
	}
	found := map[string]bool{}
	for _, action := range m.filteredActions() {
		found[action.Value] = true
		found[action.ID] = true
	}
	if found["main"] || !found["existing"] || !found["newBranch"] {
		t.Fatal("branch picker is incomplete")
	}
	awaitGit(t, m, m.execute(Action{ID: "switchBranch", Value: "existing"}))
	if m.gitError != "" || git.Inspect(p.Path).Branch != "existing" {
		t.Fatalf("switch failed: %v", m.gitError)
	}
	m.execute(Action{ID: "newBranch"})
	m.query.SetValue("new-feature")
	awaitGit(t, m, m.submitPrompt())
	if m.gitError != "" || git.Inspect(p.Path).Branch != "new-feature" {
		t.Fatalf("create and switch failed: %v", m.gitError)
	}
	awaitGit(t, m, m.switchBranch(p, "-invalid", true))
	if m.gitError == "" || git.Inspect(p.Path).Branch != "new-feature" {
		t.Fatal("invalid branch changed repository")
	}
	if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(m.execute(Action{ID: "commit"})())
	if m.mode != stageConfirm || git.Inspect(p.Path).Staged != 0 {
		t.Fatal("unstaged commit did not request approval")
	}
	m.modalKey(press('n', "n"))
	if m.mode != home || git.Inspect(p.Path).Staged != 0 {
		t.Fatal("declining staged files")
	}
	m.Update(m.execute(Action{ID: "commit"})())
	m.modalKey(press('y', "y"))
	if m.mode != prompt || !m.commitAutoStage {
		t.Fatal("stage approval did not lead to commit message")
	}
	m.query.SetValue("Staged automatically")
	m.cfg.Settings.Launch = map[string]string{"cli": "same"}
	awaitGit(t, m, m.submitPrompt())
	if m.gitError != "" {
		t.Fatal(m.gitError)
	}
	status := git.Inspect(p.Path)
	if status.Dirty() || status.Commit != "Staged automatically" || !strings.Contains(status.Graph, "Staged automatically") {
		t.Fatal("auto stage and commit failed")
	}
	if err := os.WriteFile(file, []byte("staged change"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "--all")
	if err := os.WriteFile(filepath.Join(p.Path, "leave-unstaged.txt"), []byte("unstaged"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(m.execute(Action{ID: "commit"})())
	if m.mode != prompt || m.commitAutoStage {
		t.Fatal("existing staging should go straight to commit")
	}
	m.query.SetValue("Only staged file")
	awaitGit(t, m, m.submitPrompt())
	if m.gitError != "" {
		t.Fatal(m.gitError)
	}
	if git.Inspect(p.Path).Untracked != 1 {
		t.Fatal("preexisting staging included unstaged files")
	}
	for _, action := range m.gitActions() {
		if action.ID == "lazygit" || action.Detail != "" {
			t.Fatal("uninstalled lazygit or descriptions shown")
		}
	}
	m.available["lazygit"] = true
	hasLazygit := false
	for _, action := range m.gitActions() {
		hasLazygit = hasLazygit || action.ID == "lazygit"
	}
	if !hasLazygit {
		t.Fatal("installed lazygit hidden")
	}
}

func awaitGit(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for m.mode == gitOutput && m.gitRunning {
		if time.Now().After(deadline) {
			t.Fatal("Git operation timed out")
		}
		if cmd != nil {
			cmd = m.gitProgress(cmd().(gitRunMsg))
		} else {
			time.Sleep(time.Millisecond)
			cmd = m.gitProgress(gitRunMsg{job: m.gitJobID, state: m.manager.Snapshot(m.gitJobID)})
		}
	}
}

func TestGitGraphSelectionAndCommitURL(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	s := m.statuses[p.ID]
	s.GraphLines = []git.GraphLine{{Text: "* newest", Hash: strings.Repeat("a", 40)}, {Text: "|\\"}, {Text: "| * older", Hash: strings.Repeat("b", 40)}}
	m.statuses[p.ID] = s
	m.key(press(tea.KeyEnter, ""))
	m.key(press(tea.KeyRight, ""))
	if !m.graphFocused {
		t.Fatal("Right did not focus graph")
	}
	m.key(press(tea.KeyDown, ""))
	if m.graphIndex != 2 {
		t.Fatal("graph did not skip connector row")
	}
	url, err := m.graphCommitURL()
	if err != nil || url != "https://github.com/FatalMistake02/Cardly/commit/"+strings.Repeat("b", 40) {
		t.Fatalf("commit URL: %s %v", url, err)
	}
	refreshed := s
	refreshed.GraphLines = append([]git.GraphLine{{Text: "* added", Hash: strings.Repeat("c", 40)}}, s.GraphLines...)
	m.Update(gitMsg{id: p.ID, status: refreshed})
	if m.graphIndex != 3 {
		t.Fatal("refresh changed the selected commit")
	}
	m.key(press('j', "j"))
	if m.graphIndex != 1 {
		t.Fatal("j did not move to previous commit")
	}
	view := plain(m.homeView())
	if !strings.Contains(view, "Enter open on GitHub") {
		t.Fatal("commit open hint missing")
	}
	if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
		t.Fatal("selectable graph overflows")
	}
	m.key(press(tea.KeyEscape, ""))
	if m.graphFocused || !m.projectFull {
		t.Fatal("Esc did not return focus to project")
	}
	m.key(press(tea.KeyEscape, ""))
	if m.projectFull {
		t.Fatal("Esc did not return to list")
	}
}

func TestGitOutputLifecycle(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	cmd := m.runGit(p, "Failing action", [][]string{{"switch", "--", "missing-branch"}})
	if m.mode != gitOutput || !m.gitRunning {
		t.Fatal("Git output did not open immediately")
	}
	// A tick before the process starts must not finish the operation.
	m.Update(tickMsg(time.Now()))
	if m.mode != gitOutput || !m.gitRunning {
		t.Fatal("operation completed before its start message")
	}
	awaitGit(t, m, cmd)
	if m.mode != gitOutput || m.gitError == "" || !strings.Contains(plain(m.viewport.View()), "fatal") {
		t.Fatal("failed Git action did not retain output")
	}
	m.key(press(tea.KeyEscape, ""))
	if m.mode != home {
		t.Fatal("Esc did not close failed action")
	}
	cmd = m.runGit(p, "Version", [][]string{{"--version"}})
	awaitGit(t, m, cmd)
	if m.mode != home || m.gitError != "" || !strings.Contains(m.gitLog, "git version") {
		t.Fatal("successful action did not close")
	}
}

func TestOverviewLandingAndNavigation(t *testing.T) {
	fixture := fixture(t)
	m := New(fixture.store, process.New(), log.New(io.Discard, "", 0))
	t.Cleanup(m.manager.Close)
	m.width, m.height = 120, 36
	m.startupDir = fixture.cfg.Projects[0].Path
	fixture.cfg.Settings.ProjectOpening = "normal"
	m.Update(readyMsg{cfg: fixture.cfg, available: fixture.available})
	if !m.overview || m.projectFull {
		t.Fatal("normal project-folder opening must use the default page")
	}
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.resize()
		view := plain(m.View().Content)
		if !strings.Contains(view, "OVERVIEW") || !strings.Contains(view, "Projects") || !strings.Contains(view, "Settings") {
			t.Fatal("overview page or navigation missing")
		}
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("overview overflows at %v", size)
		}
	}
	m.key(press(tea.KeyDown, ""))
	m.key(press(tea.KeyEnter, ""))
	if m.mode != settings || m.settingsPage != "" {
		t.Fatal("Settings item did not open main settings")
	}
	m.settingsKey("esc")
	if m.mode != home || !m.overview {
		t.Fatal("closing settings did not return to overview")
	}
	m.key(press(tea.KeyUp, ""))
	m.key(press(tea.KeyEnter, ""))
	if m.overview || m.projectFull {
		t.Fatal("Projects item did not open project list")
	}
	m.key(press(tea.KeyEscape, ""))
	if !m.overview {
		t.Fatal("Esc did not return to overview")
	}
	m.startSettings()
	m.openSettingsPage("behavior")
	m.settingsKey("right")
	if m.cfg.Settings.StartPage() != "projects" {
		t.Fatal("default opening page did not change")
	}
	m.settingsKey("ctrl+c")
	if m.cfg.Settings.StartPage() != "overview" {
		t.Fatal("cancel did not restore opening page preference")
	}
	cfg := fixture.cfg
	cfg.Settings.DefaultPage = "projects"
	m.Update(readyMsg{cfg: cfg, available: fixture.available})
	if m.overview || m.projectFull {
		t.Fatal("Projects opening preference did not open the list")
	}
	cfg.Projects = nil
	cfg.Settings.DefaultPage = "overview"
	m.Update(readyMsg{cfg: cfg, available: fixture.available})
	if !strings.Contains(plain(m.homeView()), "add project") {
		t.Fatal("empty overview does not offer adding a project")
	}
}

func TestOverviewShowsOnlyActionableIssues(t *testing.T) {
	m := fixture(t)
	m.overview = true
	p := m.cfg.Projects[0]
	other := m.cfg.Projects[1]
	m.statuses[other.ID] = git.Status{Error: "This directory is no longer available"}
	m.statuses[m.cfg.Projects[2].ID] = git.Status{Repository: true, Branch: "main"}
	view := plain(m.overviewView(76, 30))
	if !strings.Contains(view, "Welcome to Rian") || !strings.Contains(view, "NEEDS ATTENTION") || !strings.Contains(view, "Uncommitted changes") || !strings.Contains(view, "1 to push") {
		t.Fatal("overview lacks welcome or actionable Git issues")
	}
	for _, unwanted := range []string{"YOUR PROJECTS", "PINNED", "commands", " pinned", m.cfg.Projects[2].Name} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("unnecessary overview content: %s", unwanted)
		}
	}
	m.key(press(tea.KeyRight, ""))
	if !m.overviewFocused {
		t.Fatal("Right did not focus attention")
	}
	m.key(press(tea.KeyDown, ""))
	m.key(press(tea.KeyEnter, ""))
	current, _ := m.current()
	if m.overview || !m.projectFull || current.ID != other.ID {
		t.Fatal("selected issue did not open its project")
	}
	m.overview, m.projectFull = true, false
	m.statuses[p.ID] = git.Status{Repository: true, Branch: "main"}
	m.statuses[other.ID] = git.Status{Repository: true, Branch: "main"}
	view = plain(m.overviewView(76, 30))
	if !strings.Contains(view, "All caught up.") || strings.Contains(view, p.Name) {
		t.Fatal("clean projects should not appear as an extra project list")
	}
	delete(m.statuses, other.ID)
	view = plain(m.overviewView(76, 30))
	if strings.Contains(view, "All caught up.") || !strings.Contains(view, "Checking project status") {
		t.Fatal("unknown project status was reported as clean")
	}
}

func TestNormalAndProjectFolderOpeningAreIndependent(t *testing.T) {
	for _, page := range []string{"overview", "projects"} {
		for _, folder := range []string{"project", "normal"} {
			for _, inside := range []bool{false, true} {
				m := fixture(t)
				m.cfg.Settings.DefaultPage = page
				m.cfg.Settings.ProjectOpening = folder
				if inside {
					m.startupDir = m.cfg.Projects[0].Path
				} else {
					m.startupDir = t.TempDir()
				}
				m.Update(readyMsg{cfg: m.cfg, available: m.available})
				full := inside && folder == "project"
				if m.projectFull != full || m.overview != (!full && page == "overview") {
					t.Fatalf("page=%s folder=%s inside=%v: overview=%v full=%v", page, folder, inside, m.overview, m.projectFull)
				}
			}
		}
	}
	m := fixture(t)
	m.startSettings()
	m.openSettingsPage("behavior")
	for i, row := range m.settingsMatches() {
		if row.id == "project-startup" {
			m.settingsIndex = i
		}
	}
	m.settingsKey("right")
	if m.cfg.Settings.ProjectLaunchMode() != "normal" || m.cfg.Settings.StartPage() != "overview" {
		t.Fatal("project preference changed normal opening")
	}
	m.settingsKey("ctrl+c")
	if m.cfg.Settings.ProjectLaunchMode() != "project" || m.cfg.Settings.StartPage() != "overview" {
		t.Fatal("cancel failed to restore independent preferences")
	}
}

func TestGitGraphScopeLabelsAndControls(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	if _, err := m.store.Update(func(cfg *config.Config) error { cfg.Projects = m.cfg.Projects; return nil }); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("a", 40)
	remote := strings.Repeat("b", 40)
	feature := strings.Repeat("c", 40)
	mainRow := git.GraphLine{Hash: head, Current: true, ShortHash: "aaaaaaa", Subject: "Local work", Cells: []git.GraphCell{{Glyph: '●', Color: 0}, {Glyph: ' '}}, Refs: []git.GraphRef{{Name: "main", Kind: "local", Current: true}, {Name: "same-tip", Kind: "local"}, {Name: "v1", Kind: "tag"}}}
	remoteRow := git.GraphLine{Hash: remote, ShortHash: "bbbbbbb", Subject: "Origin work", Cells: []git.GraphCell{{Glyph: '│', Color: 0}, {Glyph: ' '}, {Glyph: '●', Color: 1}, {Glyph: ' '}}, Refs: []git.GraphRef{{Name: "origin/main", Kind: "remote"}}}
	featureRow := git.GraphLine{Hash: feature, ShortHash: "ccccccc", Subject: "Feature work", Cells: []git.GraphCell{{Glyph: '●', Color: 2}, {Glyph: ' '}}, Refs: []git.GraphRef{{Name: "feature", Kind: "local"}}}
	s := m.statuses[p.ID]
	s.Head, s.Upstream, s.Ahead, s.Behind = head, "origin/main", 1, 1
	s.CurrentGraphLines = []git.GraphLine{mainRow, remoteRow}
	s.GraphLines = []git.GraphLine{featureRow, mainRow, remoteRow}
	m.setGitStatus(p.ID, s)
	m.projectFull, m.graphFocused = true, true
	view := plain(m.gitGraphView(50, 26))
	if !strings.Contains(view, "╋") || strings.Contains(view, "●") {
		t.Fatal("commit nodes do not have connected junction markers")
	}
	for _, label := range []string{"◉ local: main", "remote: origin/main", "HEAD · local: main", "↑ 1 ahead", "Origin work"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing graph label %q: %s", label, view)
		}
	}
	if strings.Contains(view, "Feature work") {
		t.Fatal("current view includes other branch history")
	}
	footer := strings.Split(plain(m.homeView()), "\n")
	if !strings.HasPrefix(strings.TrimSpace(footer[len(footer)-1]), "Esc") {
		t.Fatal("Esc is not the first control")
	}
	_, cmd := m.graphKey("b")
	m.Update(cmd())
	if m.cfg.Settings.GitGraphScope() != "all" || m.selectedGraphHash() != head {
		t.Fatal("scope toggle lost selected HEAD")
	}
	if !strings.Contains(plain(m.gitGraphView(50, 26)), "Feature work") {
		t.Fatal("all view does not show other branch")
	}
	saved, err := m.store.Load()
	if err != nil || saved.Settings.GitGraphScope() != "all" {
		t.Fatal("graph preference did not persist")
	}
	for _, width := range []int{18, 30, 50} {
		rendered := m.gitGraphView(width, 26)
		if lipgloss.Width(rendered) > width || lipgloss.Height(rendered) > 26 {
			t.Fatal("graph labels overflow")
		}
	}
	m.startSettings()
	m.openSettingsPage("behavior")
	for i, row := range m.settingsMatches() {
		if row.id == "graph-scope" {
			m.settingsIndex = i
		}
	}
	m.settingsKey("right")
	if m.cfg.Settings.GitGraphScope() != "current" {
		t.Fatal("settings scope did not apply live")
	}
	m.settingsKey("ctrl+c")
	if m.cfg.Settings.GitGraphScope() != "all" {
		t.Fatal("cancel did not restore graph scope")
	}
	m.graphFocused = false
	last := strings.Split(plain(m.homeView()), "\n")
	if !strings.HasPrefix(strings.TrimSpace(last[len(last)-1]), "Esc projects   → Git graph") {
		t.Fatal("project controls do not start with Esc then graph")
	}
	t.Log("All-branch graph:\n" + plain(m.gitGraphView(50, 26)))
}
