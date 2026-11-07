package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rian/internal/addon"
	"rian/internal/git"
)

func TestAddonOverviewPageNavigationAndSettings(t *testing.T) {
	m := fixture(t)
	m.overview = true
	item := testAddon()
	item.Definition.Pages = []addon.Page{{ID: "tools", Name: "Tools", Title: "Project tools", Text: "Use these tools on a matching project.", Actions: []string{"open"}}}
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	if len(m.overviewPageNames()) != 3 {
		t.Fatal("addon page absent from sidebar")
	}
	m.overviewKey("down")
	m.overviewKey("down")
	if m.overviewIndex != 2 {
		t.Fatal("sidebar did not move to addon")
	}
	page, ok := m.selectedAddonPage()
	if !ok || len(m.addonPageActions(page)) != 3 {
		t.Fatal("page actions missing")
	}
	_, cmd := m.overviewKey("right")
	if cmd != nil || !m.overviewFocused {
		t.Fatal("page focusing executed an action")
	}
	if !strings.Contains(plain(m.overviewView(70, 24)), "Project tools") {
		t.Fatal("page content missing")
	}
	m.overviewKey("down")
	m.overviewKey("left")
	if m.overviewFocused {
		t.Fatal("left did not return to sidebar")
	}
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.resize()
		view := m.View().Content
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("addon page overflows %v", size)
		}
	}
	m.startSettings()
	m.openSettingsPage("addons/test")
	var row settingRow
	for _, r := range m.settingsMatches() {
		if r.operation == "page" {
			row = r
		}
	}
	if row.id == "" {
		t.Fatal("page Show/Hide missing")
	}
	m.changeAddonSetting(row, "right")
	if len(m.overviewPageNames()) != 2 {
		t.Fatal("page Hide did not apply")
	}
	m.settingsKey("ctrl+c")
	if len(m.overviewPageNames()) != 3 {
		t.Fatal("cancel did not restore page visibility")
	}
	item = m.cfg.Settings.Addons["test"]
	item.Enabled = false
	m.cfg.Settings.Addons["test"] = item
	if len(m.overviewPageNames()) != 2 {
		t.Fatal("disabled addon page remained")
	}
}
func TestAddonPageActionContextAndDetection(t *testing.T) {
	m := fixture(t)
	item := testAddon()
	item.Definition.Pages = []addon.Page{{ID: "docs", Name: "Docs", Detect: addon.Detection{Files: []string{"page.marker"}}, Sections: []addon.PageSection{{Title: "Resources", Actions: []string{"open"}}}}}
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	m.overview = true
	m.overviewIndex = 2
	page, _ := m.selectedAddonPage()
	if len(m.addonPageActions(page)) != 0 {
		t.Fatal("page ignored detection")
	}
	p := m.cfg.Projects[0]
	if err := os.WriteFile(filepath.Join(p.Path, "page.marker"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	rows := m.addonPageActions(page)
	if len(rows) != 1 || rows[0].project.ID != p.ID || rows[0].action.ID != "addon-action/test/open" {
		t.Fatal("page lost action/project context")
	}
	m.cfg.Settings.Hidden = map[string]bool{"addon/test": true}
	if len(m.addonPageActions(page)) != 0 {
		t.Fatal("page ignored addon Hide")
	}
}

func TestAddonOverviewKeepsSelectionVisible(t *testing.T) {
	m := fixture(t)
	m.overview = true
	item := testAddon()
	for i := range 30 {
		item.Definition.Pages = append(item.Definition.Pages, addon.Page{
			ID: fmt.Sprintf("page-%d", i), Name: fmt.Sprintf("Page %02d", i),
		})
	}
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.overviewKey("end")
		view := plain(m.View().Content)
		if !strings.Contains(view, "› Page 29") {
			t.Fatalf("last sidebar selection hidden at %v: %s", size, view)
		}
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("sidebar overflows %v", size)
		}
		m.overviewKey("home")
		if !strings.Contains(plain(m.View().Content), "› Projects") {
			t.Fatalf("first sidebar selection hidden at %v", size)
		}
	}
}

func TestAddonPageLongDescriptionKeepsActionsVisible(t *testing.T) {
	m := fixture(t)
	m.overview, m.overviewIndex = true, 2
	item := testAddon()
	item.Definition.Pages = []addon.Page{{
		ID: "tools", Name: "Tools", Title: "Project tools",
		Text: strings.Repeat("Long description\n", 40), Actions: []string{"open"},
	}}
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	m.overviewKey("right")
	m.overviewKey("end")
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		view := plain(m.View().Content)
		if !strings.Contains(view, "› Rin / Open") || !strings.Contains(view, "Project tools") {
			t.Fatalf("selected addon action hidden at %v: %s", size, view)
		}
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("addon actions overflow %v", size)
		}
	}
}

func TestGitActionsReflectCurrentState(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	base := git.Status{Repository: true, Branch: "main", Attached: true, Head: "abc"}
	cases := []struct {
		name         string
		status       git.Status
		want, absent []string
	}{
		{"clean local", base, []string{"addRemote", "branch"}, []string{"commit", "git/push", "git/pull", "git/fetch", "github"}},
		{"dirty", func() git.Status { s := base; s.Untracked = 1; return s }(), []string{"commit"}, nil},
		{"first push", func() git.Status { s := base; s.Remote = "https://example.com/a.git"; return s }(), []string{"git/push", "git/fetch"}, []string{"git/pull"}},
		{"caught up", func() git.Status {
			s := base
			s.Remote = "https://example.com/a.git"
			s.Tracking = "origin/main"
			return s
		}(), []string{"git/fetch"}, []string{"git/push", "git/pull"}},
		{"behind", func() git.Status {
			s := base
			s.Remote = "https://example.com/a.git"
			s.Tracking = "origin/main"
			s.Behind = 2
			return s
		}(), []string{"git/pull"}, []string{"git/push"}},
		{"diverged", func() git.Status {
			s := base
			s.Remote = "https://example.com/a.git"
			s.Tracking = "origin/main"
			s.Behind = 2
			s.Ahead = 1
			return s
		}(), nil, []string{"git/push", "git/pull"}},
		{"conflicts", func() git.Status { s := base; s.Conflicted = true; s.Modified = 1; return s }(), []string{"addRemote"}, []string{"commit", "branch", "git/push"}},
		{"unborn", git.Status{Repository: true, Attached: true, Branch: "main"}, []string{"addRemote"}, []string{"branch", "git/push"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m.statuses[p.ID] = c.status
			found := map[string]bool{}
			for _, a := range m.gitActions() {
				key := a.ID
				if a.ID == "git" {
					key += "/" + a.Value
				}
				found[key] = true
			}
			for _, key := range c.want {
				if !found[key] {
					t.Errorf("missing %s", key)
				}
			}
			for _, key := range c.absent {
				if found[key] {
					t.Errorf("unavailable %s shown", key)
				}
			}
		})
	}
}
func TestRemoteValidationAndAvailability(t *testing.T) {
	for _, c := range []struct {
		name, target, kind string
		names              []string
		valid              bool
	}{
		{"origin", "https://github.com/example/repo.git", "existing", nil, true},
		{"origin", "git@github.com:example/repo.git", "existing", nil, true},
		{"origin", "example/repo", "private", nil, true},
		{"origin", "repo", "public", nil, true},
		{"origin", "https://github.com/example/repo.git", "existing", []string{"origin"}, false},
		{"--bad", "repo", "private", nil, false},
		{"origin", "--public", "private", nil, false},
		{"origin", "bad/owner/repo", "public", nil, false},
		{"origin", "https://example.com\nmalicious", "existing", nil, false},
	} {
		if (validateRemote(c.name, c.target, c.kind, c.names) == nil) != c.valid {
			t.Fatalf("validation mismatch: %+v", c)
		}
	}
	m := fixture(t)
	p, _ := m.current()
	m.mode = remoteMenu
	m.remoteProject = p
	m.busy = true
	m.finishRemote(remoteReadyMsg{id: p.ID, status: git.Status{Repository: true}})
	if len(m.menu) != 1 || m.menu[0].Value != "existing" {
		t.Fatal("GitHub creation offered without authentication")
	}
	m.finishRemote(remoteReadyMsg{id: p.ID, status: git.Status{Repository: true}, github: true})
	if len(m.menu) != 3 {
		t.Fatal("private/public choices missing")
	}
	m.mode = home
	m.busy = true
	m.finishRemote(remoteReadyMsg{id: p.ID})
	if m.busy {
		t.Fatal("canceling probe left app busy")
	}
}
func TestExistingRemoteOutputAndPrivatePublicCreation(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	if output, err := exec.Command("git", "-C", p.Path, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", output, err)
	}
	m.remoteProject = p
	m.startRemoteForm("existing")
	m.remoteInputs[0].SetValue("origin")
	m.remoteInputs[1].SetValue("https://example.com/project.git")
	m.remoteField = 1
	m.remoteFormKey(press(tea.KeyEnter, ""))
	if m.mode != remoteConfirm {
		t.Fatal("remote did not require confirmation")
	}
	if len(git.Inspect(p.Path).Remotes) != 0 {
		t.Fatal("remote added before confirmation")
	}
	awaitGit(t, m, m.remoteFormKey(press(tea.KeyEnter, "")))
	if git.Inspect(p.Path).Remote != "https://example.com/project.git" {
		t.Fatal("remote addition failed")
	}
	installGitHubFixture(t, false)
	for _, kind := range []string{"private", "public"} {
		m.remoteKind = kind
		m.remoteName = "new-remote"
		m.remoteTarget = "owner/repo"
		m.mode = remoteConfirm
		cmd := m.remoteFormKey(press(tea.KeyEnter, ""))
		if m.mode != gitOutput || cmd == nil {
			t.Fatal("creation did not open output window")
		}
		child, err := m.outputPrepare()
		if err != nil {
			t.Fatal(err)
		}
		expected := []string{"repo", "create", "owner/repo", "--" + kind, "--source", p.Path, "--remote", "new-remote"}
		if !reflect.DeepEqual(child.Args[1:], expected) {
			t.Fatalf("creation args: %v", child.Args)
		}
		if strings.Contains(strings.Join(child.Args, " "), "--push") {
			t.Fatal("creation implicitly pushed commits")
		}
	}
}
func TestSelectedGitCommitHighlightsWholeRow(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	row := git.GraphLine{Hash: "abc", ShortHash: "abc", Subject: "Complete commit message", CurrentBranch: true, Cells: []git.GraphCell{{Glyph: '●', Color: 0, CurrentBranch: true}}, Refs: []git.GraphRef{{Name: "main", Kind: "local", Current: true}, {Name: "origin/main", Kind: "remote"}}}
	m.statuses[p.ID] = git.Status{Repository: true, Branch: "main", GraphLines: []git.GraphLine{row}}
	m.graphFocused = true
	view := m.gitGraphView(65, 20)
	style := selectedStyle.Foreground(accent)
	for _, line := range graphRenderedRows(row, 65) {
		expected := style.Render(pad(plain(line), 65))
		if !strings.Contains(view, expected) {
			t.Fatalf("selected row retained nested style resets: %q", view)
		}
	}
}

func TestGitMenuInitializesFolderWithoutRepository(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	m.available["git"] = true
	m.statuses[p.ID] = git.Inspect(p.Path)
	actions := m.gitActionsFor(p)
	if len(actions) != 1 || actions[0].ID != "git" || actions[0].Value != "init" {
		t.Fatalf("nonrepo actions: %+v", actions)
	}
	if !strings.Contains(strings.Join(m.projectHints(p, 100), " "), "Git") {
		t.Fatal("Git menu shortcut missing without repository")
	}
	m.startSettings()
	m.openSettingsPage("project settings/" + p.ID)
	found := false
	for _, row := range m.settingsMatches() {
		found = found || row.id == "git"
	}
	if !found {
		t.Fatal("Git visibility setting missing")
	}
	m.settingsKey("ctrl+c")
	cmd := m.openMenu(gitMenu)
	if cmd == nil {
		t.Fatal("Git menu did not inspect folder")
	}
	m.Update(cmd())
	if m.mode != gitMenu || len(m.filteredActions()) != 1 || m.filteredActions()[0].Value != "init" {
		t.Fatal("menu rejected folder without git")
	}
	awaitGit(t, m, m.execute(m.filteredActions()[0]))
	status := git.Inspect(p.Path)
	if !status.Repository || m.gitError != "" || m.mode != home {
		t.Fatalf("initialization failed: %+v %s", status, m.gitError)
	}
	m.setGitStatus(p.ID, status)
	for _, a := range m.gitActionsFor(p) {
		if a.Value == "init" {
			t.Fatal("Initialize remained after repository creation")
		}
	}
	m.statuses[p.ID] = git.Status{}
	m.available["git"] = false
	if len(m.gitActionsFor(p)) != 0 || m.gitMenuAvailable(p) {
		t.Fatal("Initialize offered without installed git")
	}
	m.available["git"] = true
	m.statuses[p.ID] = git.Status{Error: "folder unavailable"}
	if len(m.gitActionsFor(p)) != 0 {
		t.Fatal("Initialize offered for inaccessible folder")
	}
}
