package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rian/internal/addon"
	"rian/internal/config"
	"rian/internal/project"
)

func addonUpdateFixture(t *testing.T) (*Model, addon.Installed, addon.Definition) {
	t.Helper()
	m := fixture(t)
	current := testAddon()
	current.Source = "/addons/test.json"
	current.Definition.UpdateURL = "https://example.com/test.json"
	current.Values["text"] = "my value"
	next := current.Definition
	next.Version = "1.1.0"
	next.Name = "Updated addon"
	next.Shortcut = "y"
	next.Pages = []addon.Page{{ID: "new-page", Name: "New page"}}
	m.cfg.Settings.Addons = map[string]addon.Installed{"test": current}
	cfg, err := m.store.Update(func(cfg *config.Config) error { *cfg = m.cfg; return nil })
	if err != nil {
		t.Fatal(err)
	}
	m.cfg = cfg
	m.addonUpdateCheck = func(context.Context, map[string]addon.Installed) map[string]addon.UpdateResult {
		return map[string]addon.UpdateResult{"test": {Definition: next}}
	}
	return m, current, next
}

func checkAddonUpdate(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.checkAddonUpdates()
	if cmd == nil {
		t.Fatal("missing background check")
	}
	m.Update(cmd())
}

func TestAddonChecksAreBackgroundThrottledAndDiscardStaleResults(t *testing.T) {
	m, current, next := addonUpdateFixture(t)
	calls := 0
	m.addonUpdateCheck = func(_ context.Context, installed map[string]addon.Installed) map[string]addon.UpdateResult {
		calls++
		if installed["test"].Definition.UpdateURL != current.Definition.UpdateURL {
			t.Fatal("background check used mutable config")
		}
		return map[string]addon.UpdateResult{"test": {Definition: next}}
	}
	cmd := m.checkAddonUpdates()
	if cmd == nil || calls != 0 || m.busy {
		t.Fatal("checking blocked input or fetched before command execution")
	}
	// A changed update source must not receive the old source's manifest.
	changed := current.Clone()
	changed.Definition.UpdateURL = "https://example.com/other.json"
	m.cfg.Settings.Addons["test"] = changed
	_, recheck := m.Update(cmd())
	if calls != 1 || len(m.pendingAddonUpdates()) != 0 || recheck == nil {
		t.Fatal("stale result was accepted or changed source was not rechecked")
	}
	// A fetch finishing after removal cannot restore the addon.
	delete(m.cfg.Settings.Addons, "test")
	m.Update(addonUpdatesMsg{installed: map[string]addon.Installed{"test": changed}, results: map[string]addon.UpdateResult{"test": {Definition: next}}})
	if len(m.pendingAddonUpdates()) != 0 || m.addonUpdatesChecking {
		t.Fatal("removed addon returned from stale check")
	}
	m.cfg.Settings.Addons["test"] = current
	m.addonUpdateCheck = func(context.Context, map[string]addon.Installed) map[string]addon.UpdateResult {
		return map[string]addon.UpdateResult{"test": {Definition: next}}
	}
	checkAddonUpdate(t, m)
	if len(m.pendingAddonUpdates()) != 1 || m.checkAddonUpdates() != nil {
		t.Fatal("update missing or same-hour check repeated")
	}
	checked := m.addonUpdatesChecked["test"]
	checked.at = time.Now().Add(-time.Hour)
	m.addonUpdatesChecked["test"] = checked
	if m.checkAddonUpdates() == nil {
		t.Fatal("hourly check did not run")
	}
}

func TestAddonUpdateMenuPersistsLatestChoices(t *testing.T) {
	m, current, _ := addonUpdateFixture(t)
	current.Enabled = false
	m.cfg.Settings.Addons["test"] = current
	if cfg, err := m.store.Update(func(cfg *config.Config) error { cfg.Settings.Addons["test"] = current; return nil }); err != nil {
		t.Fatal(err)
	} else {
		m.cfg = cfg
	}
	checkAddonUpdate(t, m)
	m.key(press('U', "U"))
	if m.mode != addonUpdates {
		t.Fatal("addon update notice did not open list")
	}
	m.modalKey(press(tea.KeyEnter, ""))
	if m.mode != addonUpdateConfirm || !strings.Contains(plain(m.modalView()), "1.0.0 → 1.1.0") {
		t.Fatal("version review missing")
	}
	// Another instance can edit values before installation. Its newest values
	// must survive, along with this disabled addon's existing source and key.
	if _, err := m.store.Update(func(cfg *config.Config) error {
		item := cfg.Settings.Addons["test"]
		item.Values["text"] = "latest value"
		cfg.Settings.Addons["test"] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cmd := m.modalKey(press(tea.KeyEnter, ""))
	if cmd == nil || !m.busy {
		t.Fatal("Update did not begin persistence")
	}
	m.Update(cmd())
	stored, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	updated := stored.Settings.Addons["test"]
	if updated.Definition.Version != "1.1.0" || updated.Enabled || updated.Source != current.Source ||
		updated.Definition.Shortcut != current.Definition.Shortcut || updated.Values["text"] != "latest value" {
		t.Fatalf("update lost installation choices: %+v", updated)
	}
	if m.busy || m.mode != home || len(m.pendingAddonUpdates()) != 0 {
		t.Fatal("successful update left stale UI state")
	}
	// The new manifest's settings/page information is available immediately.
	m.startSettings()
	m.openSettingsPage("addons/test")
	if !strings.Contains(plain(m.settingsHeader(70)), "1.1.0") || !strings.Contains(plain(m.settingsView(70)), "New page") {
		t.Fatal("updated addon settings did not refresh")
	}
}

func TestAddonUpdateRefusesRemovedOrChangedStoredAddon(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed", true: "removed"}[remove], func(t *testing.T) {
			m, _, _ := addonUpdateFixture(t)
			checkAddonUpdate(t, m)
			m.key(press('U', "U"))
			m.modalKey(press(tea.KeyEnter, ""))
			if _, err := m.store.Update(func(cfg *config.Config) error {
				if remove {
					delete(cfg.Settings.Addons, "test")
				} else {
					item := cfg.Settings.Addons["test"]
					item.Definition.Name = "Edited elsewhere"
					cfg.Settings.Addons["test"] = item
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			cmd := m.modalKey(press(tea.KeyEnter, ""))
			msg := cmd().(addonUpdatedMsg)
			if msg.err == nil {
				t.Fatal("stale review overwrote a changed or removed addon")
			}
			m.Update(msg)
			stored, err := m.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			item, exists := stored.Settings.Addons["test"]
			if remove && exists || !remove && (item.Definition.Name != "Edited elsewhere" || item.Definition.Version != "1.0.0") {
				t.Fatal("failed update changed persisted state")
			}
			if len(m.pendingAddonUpdates()) != 1 || !m.toastError || m.busy {
				t.Fatal("failed update did not retain its notice/error")
			}
		})
	}
}

func TestAddonUpdateDismissSearchAndHeaderLayout(t *testing.T) {
	m, current, next := addonUpdateFixture(t)
	checkAddonUpdate(t, m)
	m.latestRelease = "0.3.0"
	m.cfg.Settings.AutoAddEnabled = true
	m.autoAddKey = autoAddKey(m.cfg.Settings)
	m.autoAddCandidates = []project.Project{project.New("New", t.TempDir())}
	for _, size := range [][2]int{{52, 16}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		for _, full := range []bool{false, true} {
			m.projectFull = full
			m.cfg.Projects[0].Name = strings.Repeat("Long project name ", 10)
			header := strings.Split(plain(m.homeView()), "\n")[0]
			for _, key := range []string{"A ", "u ", "U ", "Ctrl+K"} {
				if !strings.Contains(header, key) {
					t.Fatalf("coexisting notice %q hidden at %v (full %t): %s", key, size, full, header)
				}
			}
			if lipgloss.Width(header) > m.width {
				t.Fatalf("header overflows %v (full %t): %s", size, full, header)
			}
		}
		m.key(press('U', "U"))
		m.modalKey(press(tea.KeyEnter, ""))
		view := m.View().Content
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height || !strings.Contains(plain(m.modalView()), "Dismiss") {
			t.Fatalf("addon update review overflows or hides a choice at %v", size)
		}
		m.closeModal()
	}
	m.searching = true
	m.search.Focus()
	m.key(press('U', "U"))
	if m.mode != home || m.search.Value() != "U" {
		t.Fatal("addon update shortcut intercepted project search")
	}
	m.key(press(tea.KeyEscape, ""))
	m.key(press('U', "U"))
	m.modalKey(press(tea.KeyEnter, ""))
	m.modalKey(press(tea.KeyDown, ""))
	m.modalKey(press(tea.KeyEnter, ""))
	if len(m.pendingAddonUpdates()) != 0 || m.mode != home {
		t.Fatal("dismiss did not clear addon notice")
	}
	m.Update(addonUpdatesMsg{installed: map[string]addon.Installed{"test": current}, results: map[string]addon.UpdateResult{"test": {Definition: next}}})
	if len(m.pendingAddonUpdates()) != 0 {
		t.Fatal("dismissed version reappeared after checking")
	}
	next.Version = "1.2.0"
	m.Update(addonUpdatesMsg{installed: map[string]addon.Installed{"test": current}, results: map[string]addon.UpdateResult{"test": {Definition: next}}})
	if len(m.pendingAddonUpdates()) != 1 {
		t.Fatal("newer version was hidden by previous dismissal")
	}
}

func TestAddonImportUpgradeCanBeCanceled(t *testing.T) {
	m, original, next := addonUpdateFixture(t)
	m.startSettings()
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Definition: next, Enabled: true, Source: "/replacement.json"}})
	upgraded := m.cfg.Settings.Addons["test"]
	if upgraded.Definition.Version != "1.1.0" || upgraded.Source != original.Source || upgraded.Definition.Shortcut != original.Definition.Shortcut || upgraded.Values["text"] != "my value" {
		t.Fatal("newer import did not preserve addon choices")
	}
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Definition: next}})
	if m.mode != addonImport {
		t.Fatal("equal version replaced addon")
	}
	m.settingsKey("ctrl+c")
	if !reflect.DeepEqual(m.cfg.Settings.Addons["test"], original) {
		t.Fatal("cancel did not restore original addon definition and values")
	}
	stored, err := m.store.Load()
	if err != nil || stored.Settings.Addons["test"].Definition.Version != "1.0.0" {
		t.Fatal("canceled import changed persisted addon")
	}
}

func TestAddonImportedUpdateRefusesConcurrentRelease(t *testing.T) {
	m, _, next := addonUpdateFixture(t)
	m.startSettings()
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Definition: next}})
	if _, err := m.store.Update(func(cfg *config.Config) error {
		item := cfg.Settings.Addons["test"]
		item.Definition.Version = "1.2.0"
		item.Values["text"] = "newer installation"
		cfg.Settings.Addons["test"] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cmd := m.keepSettings()
	m.Update(cmd())
	stored, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	item := stored.Settings.Addons["test"]
	if item.Definition.Version != "1.2.0" || item.Values["text"] != "newer installation" || !m.toastError || m.busy {
		t.Fatal("stale local import replaced a concurrent release")
	}
}

func TestAddonImportedUpdateRefusesConcurrentChoiceEdits(t *testing.T) {
	m, _, next := addonUpdateFixture(t)
	m.startSettings()
	m.mode = addonImport
	m.acceptAddon(addonImportedMsg{item: addon.Installed{Definition: next}})
	draft := m.settingsDraft.Addons["test"]
	draft.Values["toggle"] = "true"
	m.settingsDraft.Addons["test"] = draft
	m.applySettingsChange()
	if _, err := m.store.Update(func(cfg *config.Config) error {
		item := cfg.Settings.Addons["test"]
		item.Values["text"] = "latest value"
		item.Enabled = false
		item.Source = "/new-source.json"
		cfg.Settings.Addons["test"] = item
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(m.keepSettings()())
	stored, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	item := stored.Settings.Addons["test"]
	if item.Definition.Version != "1.0.0" || item.Values["text"] != "latest value" || item.Values["toggle"] != "" || item.Enabled || item.Source != "/new-source.json" || !m.toastError {
		t.Fatalf("stale local import changed the latest installation choices: %+v", item)
	}
}

func TestSettingsPreserveConcurrentAddonsOrRejectAddonEdits(t *testing.T) {
	for _, importAddon := range []bool{false, true} {
		t.Run(map[bool]string{false: "global setting", true: "addon import"}[importAddon], func(t *testing.T) {
			m, current, next := addonUpdateFixture(t)
			other := current.Clone()
			other.Definition.ID = "other"
			cfg, err := m.store.Update(func(cfg *config.Config) error {
				cfg.Settings.Addons["other"] = other
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			m.cfg = cfg
			m.startSettings()
			if importAddon {
				m.mode = addonImport
				m.acceptAddon(addonImportedMsg{item: addon.Installed{Definition: next}})
			} else {
				m.settingsDraft.Accent = "mint-green"
				m.applySettingsChange()
			}
			if _, err := m.store.Update(func(cfg *config.Config) error {
				item := cfg.Settings.Addons["other"]
				item.Definition.Version = "1.2.0"
				cfg.Settings.Addons["other"] = item
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			m.Update(m.keepSettings()())
			stored, err := m.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if stored.Settings.Addons["other"].Definition.Version != "1.2.0" || stored.Settings.Addons["test"].Definition.Version != "1.0.0" {
				t.Fatal("settings downgraded a concurrently updated addon")
			}
			if importAddon != m.toastError || !importAddon && stored.Settings.Accent != "mint-green" {
				t.Fatal("settings did not preserve untouched addons or reject stale addon edits")
			}
		})
	}
}

func TestAddonUpdateCompletionDoesNotReopenEscapedReview(t *testing.T) {
	m, current, next := addonUpdateFixture(t)
	other := current.Clone()
	other.Definition.ID = "other"
	other.Definition.UpdateURL = ""
	otherNext := other.Definition
	otherNext.Version = "1.1.0"
	cfg, err := m.store.Update(func(cfg *config.Config) error {
		cfg.Settings.Addons["other"] = other
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.cfg = cfg
	m.addonUpdatesAvailable = map[string]addonUpdate{
		"test":  {installed: current.Definition, available: next},
		"other": {installed: other.Definition, available: otherNext},
	}
	m.mode, m.addonUpdateID = addonUpdateConfirm, "test"
	cmd := m.modalKey(press(tea.KeyEnter, ""))
	m.key(press(tea.KeyEscape, ""))
	if m.mode != home || !m.busy {
		t.Fatal("Escape did not close pending review")
	}
	m.Update(cmd())
	if m.mode != home || m.busy || len(m.pendingAddonUpdates()) != 1 || m.cfg.Settings.Addons["test"].Definition.Version != "1.1.0" {
		t.Fatal("completion reopened escaped review or lost the update")
	}
}

func TestAddonCheckFailureAndSettingsCancelLeaveInstallationUntouched(t *testing.T) {
	m, current, next := addonUpdateFixture(t)
	m.addonUpdateCheck = func(context.Context, map[string]addon.Installed) map[string]addon.UpdateResult {
		return map[string]addon.UpdateResult{"test": {Err: errors.New("network unavailable")}}
	}
	checkAddonUpdate(t, m)
	if m.toastError || m.toast != "" || len(m.pendingAddonUpdates()) != 0 || !reflect.DeepEqual(m.cfg.Settings.Addons["test"], current) {
		t.Fatal("background failure changed the addon or interrupted the UI")
	}
	m.startSettings()
	item := m.settingsDraft.Addons["test"]
	item.Enabled = false
	m.settingsDraft.Addons["test"] = item
	m.applySettingsChange()
	m.Update(addonUpdatesMsg{installed: map[string]addon.Installed{"test": current}, results: map[string]addon.UpdateResult{"test": {Definition: next}}})
	m.key(press('U', "U"))
	if m.mode != settings {
		t.Fatal("addon update review interrupted settings")
	}
	m.settingsKey("ctrl+c")
	if !reflect.DeepEqual(m.cfg.Settings.Addons["test"], current) || len(m.pendingAddonUpdates()) != 1 {
		t.Fatal("background result interfered with settings cancellation")
	}
}

func TestAddonUpdateHTTPToPersistedConfig(t *testing.T) {
	m, current, next := addonUpdateFixture(t)
	manifest, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("unexpected update request: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(manifest)
	}))
	defer server.Close()
	current.Definition.UpdateURL = server.URL + "/addon.json"
	current.Enabled = false
	cfg, err := m.store.Update(func(cfg *config.Config) error {
		cfg.Settings.Addons["test"] = current
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.cfg = cfg
	m.addonUpdateCheck = nil // Exercise the real HTTP checker.
	checkAddonUpdate(t, m)
	if requests.Load() != 1 || len(m.pendingAddonUpdates()) != 1 {
		t.Fatal("HTTP manifest did not become a reviewable update")
	}
	m.key(press('U', "U"))
	m.modalKey(press(tea.KeyEnter, ""))
	cmd := m.modalKey(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("reviewed HTTP update did not begin installation")
	}
	m.Update(cmd())
	stored, err := m.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	updated := stored.Settings.Addons["test"]
	if updated.Definition.Version != "1.1.0" || updated.Enabled || updated.Values["text"] != "my value" || updated.Source != current.Source {
		t.Fatalf("HTTP update lost persisted choices: %+v", updated)
	}
}
