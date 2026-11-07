package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/addon"
	"rian/internal/config"
)

func testAddon() addon.Installed {
	return addon.Installed{Enabled: true, Definition: addon.Definition{FormatVersion: 1, Version: "1.0.0", ID: "test", Name: "Test addon", Menu: true, Shortcut: "z", Settings: []addon.Setting{{ID: "toggle", Name: "Feature", Type: "bool", Default: false}, {ID: "text", Name: "Message", Type: "text", Default: "hello"}}, Actions: []addon.Action{{ID: "open", Name: "Open", Kind: "url", URL: "https://example.com"}}}, Values: map[string]string{}}
}
func TestAddonSettingsImportCancelAndPersistence(t *testing.T) {
	m := fixture(t)
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": testAddon()}
	m.startSettings()
	m.openSettingsPage("addons/test")
	rows := m.settingsMatches()
	if len(rows) != 5 || rows[0].label != "Enabled" || rows[4].operation != "remove" {
		t.Fatalf("addon page: %+v", rows)
	}
	m.changeAddonSetting(rows[2], "right")
	if m.cfg.Settings.Addons["test"].Values["toggle"] != "true" {
		t.Fatal("toggle did not apply immediately")
	}
	m.changeAddonSetting(rows[3], "enter")
	m.query.SetValue("updated")
	m.addonFormKey(press(tea.KeyEnter, ""))
	if m.cfg.Settings.Addons["test"].Values["text"] != "updated" {
		t.Fatal("text setting did not apply")
	}
	m.changeAddonSetting(rows[4], "enter")
	m.addonFormKey(press(tea.KeyEnter, ""))
	if len(m.cfg.Settings.Addons) != 0 {
		t.Fatal("remove failed")
	}
	m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if len(m.cfg.Settings.Addons) != 1 || m.cfg.Settings.Addons["test"].Value(testAddon().Definition.Settings[1]) != "hello" {
		t.Fatal("settings cancel did not restore addon snapshot")
	}
	m.startSettings()
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Enabled: true, Definition: addon.Definition{FormatVersion: 1, Version: "1.0.0", ID: "other", Name: "Other", Actions: testAddon().Definition.Actions}}})
	if len(m.cfg.Settings.Addons) != 2 || m.settingsPage != "addons/other" {
		t.Fatal("import did not apply")
	}
	m.settingsKey("ctrl+c")
	if len(m.cfg.Settings.Addons) != 1 {
		t.Fatal("cancel did not undo import")
	}
	if _, err := m.store.Update(func(c *config.Config) error {
		c.Projects = m.cfg.Projects
		c.Settings = m.cfg.Settings.Clone()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.startSettings()
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Enabled: true, Definition: addon.Definition{FormatVersion: 1, Version: "1.0.0", ID: "other", Name: "Other", Actions: testAddon().Definition.Actions}}})
	cmd := m.keepSettings()
	if cmd == nil {
		t.Fatal("missing save")
	}
	m.Update(cmd())
	cfg, err := m.store.Load()
	if err != nil || len(cfg.Settings.Addons) != 2 {
		t.Fatalf("addon persistence: %v %+v", err, cfg.Settings.Addons)
	}
}
func TestAddonAvailabilityShortcutAndSearch(t *testing.T) {
	m := fixture(t)
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": testAddon()}
	p, _ := m.current()
	if len(m.addonActions(p)) != 1 {
		t.Fatal("addon absent")
	}
	handled, _ := m.addonShortcut("z")
	if !handled || m.mode != addonMenu || len(m.filteredActions()) != 1 {
		t.Fatal("shortcut did not open actions")
	}
	m.closeModal()
	if !strings.Contains(strings.Join(m.projectHints(p, 100), " "), "Test addon") {
		t.Fatal("shortcut hint absent")
	}
	m.startSettings()
	m.settingsSearch.SetValue("Message")
	rows := m.settingsMatches()
	if len(rows) != 1 || rows[0].section != "addons/test" || !strings.Contains(m.settingsView(70), "addons / Test addon") {
		t.Fatal("recursive addon settings search failed")
	}
	m.openSettingsPage("addons/test")
	m.settingsKey("esc")
	if m.settingsPage != "addons" {
		t.Fatal("addon Esc skipped parent")
	}
	m.settingsKey("ctrl+c")
	item := m.cfg.Settings.Addons["test"]
	for _, key := range []string{"p", "A", "u", "U", "G", "K"} {
		item.Definition.Shortcut = key
		m.cfg.Settings.Addons["test"] = item
		if m.addonKey("test") != "" {
			t.Fatalf("addon took built-in key %q", key)
		}
	}
	m.cfg.Settings.Overrides = map[string]map[string]bool{p.ID: {"addon/test": false}}
	if len(m.addonActions(p)) != 0 {
		t.Fatal("project Hide ignored")
	}
	m.cfg.Settings.Overrides = nil
	item.Enabled = false
	m.cfg.Settings.Addons["test"] = item
	if len(m.addonActions(p)) != 0 {
		t.Fatal("disabled addon visible")
	}
	m.startSettings()
	m.openSettingsPage("addons/test")
	if len(m.settingsMatches()) != 5 {
		t.Fatal("disabled addon not configurable")
	}
}

func TestCShortcutIsAvailableToAddons(t *testing.T) {
	m := fixture(t)
	if cmd := m.key(press('c', "c")); cmd != nil || m.mode != home {
		t.Fatal("c still triggered a built-in action")
	}
	item := testAddon()
	item.Definition.Shortcut = "c"
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	if m.addonKey("test") != "c" {
		t.Fatal("c is still reserved")
	}
	m.key(press('c', "c"))
	if m.mode != addonMenu || len(m.filteredActions()) != 1 {
		t.Fatal("c did not open the addon")
	}
}
func TestAddonImportFailureAndPickerCancel(t *testing.T) {
	m := fixture(t)
	m.startSettings()
	m.mode = addonImport
	m.query = input("path")
	m.addonLoading = true
	m.acceptAddon(addonImportedMsg{canceled: true})
	if m.mode != addonImport || m.addonLoading || !m.query.Focused() {
		t.Fatal("picker cancel left input unusable")
	}
	m.acceptAddon(addonImportedMsg{err: os.ErrNotExist})
	if m.mode != addonImport || !m.query.Focused() {
		t.Fatal("failed import closed input")
	}
	m.acceptAddon(addonImportedMsg{item: testAddon()})
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: testAddon()})
	if len(m.cfg.Settings.Addons) != 1 || m.mode != addonImport {
		t.Fatal("duplicate import replaced addon")
	}
}
func TestAddonOutputSuccessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			m := fixture(t)
			defer m.manager.Close()
			p, _ := m.current()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := m.runAddonOutput(p, "Test output", func() (*exec.Cmd, error) {
				child := exec.Command(binary, "-test.run=TestAddonOutputChild")
				child.Dir = p.Path
				child.Env = append(os.Environ(), "RIAN_ADDON_TEST_CHILD=1")
				if fail {
					child.Env = append(child.Env, "RIAN_ADDON_TEST_FAIL=1")
				}
				return child, nil
			})
			msg := cmd().(gitRunMsg)
			m.gitProgress(msg)
			deadline := time.Now().Add(5 * time.Second)
			for m.gitRunning && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				m.gitProgress(gitRunMsg{job: m.gitJobID, state: m.manager.Snapshot(m.gitJobID)})
			}
			if m.gitRunning {
				t.Fatal("output did not complete")
			}
			if fail {
				if m.mode != gitOutput || m.gitError == "" || !strings.Contains(plain(m.viewport.View()), "addon child output") {
					t.Fatal("failed output not retained")
				}
				m.modalKey(press(tea.KeyEscape, ""))
				if m.mode != home {
					t.Fatal("failed output did not close")
				}
			} else if m.mode != home {
				t.Fatal("successful output did not auto-close")
			}
		})
	}
}
func TestAddonOutputChild(t *testing.T) {
	if os.Getenv("RIAN_ADDON_TEST_CHILD") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString("addon child output\n")
	if os.Getenv("RIAN_ADDON_TEST_FAIL") == "1" {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestAddonImportFromPath(t *testing.T) {
	m := fixture(t)
	m.startSettings()
	m.mode = addonImport
	m.query = input("path")
	path, err := filepath.Abs(filepath.Join("..", "..", "examples", "addons", "npm.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.query.SetValue(path)
	cmd := m.addonFormKey(press(tea.KeyEnter, ""))
	m.Update(cmd())
	if _, ok := m.cfg.Settings.Addons["npm"]; !ok {
		t.Fatal("file import failed")
	}
}

func TestAddonCommandsAndSingleAction(t *testing.T) {
	m := fixture(t)
	p, _ := m.current()
	item := testAddon()
	item.Definition.Actions[0].Kind = "output"
	item.Definition.Actions[0].Executable = "rian-addon-missing"
	item.Definition.Actions[0].URL = ""
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": item}
	m.openMenu(commands)
	for _, a := range m.menu {
		if strings.HasPrefix(a.ID, "addon-action/") {
			t.Fatal("missing executable offered as command")
		}
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(binary))
	item.Definition.Actions[0].Executable = filepath.Base(binary)
	m.cfg.Settings.Addons["test"] = item
	m.openMenu(commands)
	found := false
	for _, a := range m.menu {
		found = found || a.ID == "addon-action/test/open"
	}
	if !found {
		t.Fatal("detected addon command absent from Commands menu")
	}
	item.Definition.Menu = false
	m.cfg.Settings.Addons["test"] = item
	cmd := m.executeAddon(Action{ID: "addon/test"}, p)
	if m.mode != gitOutput || cmd == nil {
		t.Fatal("single action did not open directly")
	}
}
