package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/addon"
	"rian/internal/config"
)

type addonChecker func(context.Context, map[string]addon.Installed) map[string]addon.UpdateResult

type addonUpdate struct {
	installed addon.Definition
	available addon.Definition
}

type addonUpdateCheckState struct {
	key string
	at  time.Time
}

type addonUpdatesMsg struct {
	installed map[string]addon.Installed
	results   map[string]addon.UpdateResult
}

type addonUpdatedMsg struct {
	id  string
	cfg config.Config
	err error
}

func addonUpdateKey(d addon.Definition) string { return d.Version + "\x00" + d.UpdateURL }

// Check only fetches manifests. Installing a newer definition requires a
// separate user selection and an atomic write against the latest config.
func (m *Model) checkAddonUpdates() tea.Cmd {
	if m.addonUpdatesChecking {
		return nil
	}
	if m.addonUpdatesChecked == nil {
		m.addonUpdatesChecked = map[string]addonUpdateCheckState{}
	}
	for id := range m.addonUpdatesChecked {
		if _, installed := m.cfg.Settings.Addons[id]; !installed {
			delete(m.addonUpdatesChecked, id)
			delete(m.addonUpdatesAvailable, id)
		}
	}
	installed := map[string]addon.Installed{}
	now := time.Now()
	for id, item := range m.cfg.Settings.Addons {
		if item.Definition.UpdateURL == "" {
			continue
		}
		key := addonUpdateKey(item.Definition)
		checked := m.addonUpdatesChecked[id]
		if checked.key == key && now.Sub(checked.at) < time.Hour {
			continue
		}
		installed[id] = item.Clone()
		m.addonUpdatesChecked[id] = addonUpdateCheckState{key, now}
	}
	if len(installed) == 0 {
		return nil
	}
	m.addonUpdatesChecking = true
	check := m.addonUpdateCheck
	if check == nil {
		check = addon.CheckUpdates
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return addonUpdatesMsg{installed, check(ctx, installed)}
	}
}

func (m *Model) acceptAddonUpdates(msg addonUpdatesMsg) tea.Cmd {
	m.addonUpdatesChecking = false
	if m.addonUpdatesAvailable == nil {
		m.addonUpdatesAvailable = map[string]addonUpdate{}
	}
	for id, result := range msg.results {
		before, checked := msg.installed[id]
		current, installed := m.cfg.Settings.Addons[id]
		if !checked || !installed || addonUpdateKey(before.Definition) != addonUpdateKey(current.Definition) {
			continue
		}
		if result.Err != nil {
			if m.logger != nil {
				m.logger.Printf("Check addon %s: %v", id, result.Err)
			}
			continue
		}
		if result.Definition.ID == id && addon.Newer(result.Definition.Version, current.Definition.Version) {
			m.addonUpdatesAvailable[id] = addonUpdate{current.Definition, result.Definition}
		} else {
			delete(m.addonUpdatesAvailable, id)
		}
	}
	// Changes made while a fetch was running get their own check immediately.
	return m.checkAddonUpdates()
}

func (m *Model) pendingAddonUpdates() []addonUpdate {
	items := []addonUpdate{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		update, ok := m.addonUpdatesAvailable[id]
		current := m.cfg.Settings.Addons[id]
		if ok && addonUpdateKey(current.Definition) == addonUpdateKey(update.installed) &&
			addon.Newer(update.available.Version, current.Definition.Version) &&
			m.addonUpdatesDismissed[id] != update.available.Version {
			items = append(items, update)
		}
	}
	return items
}

func (m *Model) addonUpdatesKey(key string) tea.Cmd {
	items := m.pendingAddonUpdates()
	if m.mode == addonUpdateConfirm {
		update, ok := m.addonUpdatesAvailable[m.addonUpdateID]
		current, installed := m.cfg.Settings.Addons[m.addonUpdateID]
		if !ok || !installed || addonUpdateKey(current.Definition) != addonUpdateKey(update.installed) {
			m.mode = addonUpdates
			return nil
		}
		switch key {
		case "esc":
			m.mode = addonUpdates
		case "up", "j":
			m.addonUpdateChoice = 0
		case "down", "k":
			m.addonUpdateChoice = 1
		case "enter":
			if m.addonUpdateChoice == 1 {
				if m.addonUpdatesDismissed == nil {
					m.addonUpdatesDismissed = map[string]string{}
				}
				m.addonUpdatesDismissed[m.addonUpdateID] = update.available.Version
				m.mode = addonUpdates
				if len(m.pendingAddonUpdates()) == 0 {
					m.closeModal()
				}
				return nil
			}
			return m.installAddonUpdate(current, update.available)
		}
		return nil
	}
	m.menuIndex = min(m.menuIndex, max(0, len(items)-1))
	switch key {
	case "esc":
		m.closeModal()
	case "up", "j":
		m.menuIndex = max(0, m.menuIndex-1)
	case "down", "k":
		m.menuIndex = min(max(0, len(items)-1), m.menuIndex+1)
	case "enter":
		if len(items) > 0 {
			m.addonUpdateID = items[m.menuIndex].installed.ID
			m.addonUpdateChoice = 0
			m.mode = addonUpdateConfirm
		}
	}
	return nil
}

func (m *Model) installAddonUpdate(reviewed addon.Installed, next addon.Definition) tea.Cmd {
	m.busy = true
	store, id := m.store, reviewed.Definition.ID
	return func() tea.Msg {
		cfg, err := store.Update(func(cfg *config.Config) error {
			current, exists := cfg.Settings.Addons[id]
			if !exists || !reflect.DeepEqual(current.Definition, reviewed.Definition) {
				return fmt.Errorf("addon changed since review; reopen Rian to check its latest version")
			}
			upgraded, err := current.Upgrade(next)
			if err != nil {
				return err
			}
			cfg.Settings.Addons[id] = upgraded
			return nil
		})
		return addonUpdatedMsg{id, cfg, err}
	}
}

func (m *Model) acceptAddonUpdate(msg addonUpdatedMsg) tea.Cmd {
	m.busy = false
	if msg.err != nil {
		return m.notify("Could not update addon", msg.err)
	}
	if msg.cfg.Revision >= m.cfg.Revision {
		m.cfg = msg.cfg
		m.applyAppearance()
	}
	delete(m.addonUpdatesAvailable, msg.id)
	m.menuIndex = min(m.menuIndex, max(0, len(m.pendingAddonUpdates())-1))
	if m.mode == addonUpdates || m.mode == addonUpdateConfirm {
		m.mode = addonUpdates
		if len(m.pendingAddonUpdates()) == 0 {
			m.closeModal()
		}
	}
	return tea.Batch(m.checkAddonUpdates(), m.notify("Addon updated", nil))
}

func (m *Model) addonUpdatesView(width int) (string, string, string) {
	footer := keyHint("Esc", "back") + "  " + keyHint("↑↓", "choose") + "  " + keyHint("Enter", "select")
	if m.mode == addonUpdateConfirm {
		update, ok := m.addonUpdatesAvailable[m.addonUpdateID]
		if !ok {
			return "ADDON UPDATE", "This update is no longer available.", footer
		}
		lines := []string{fit(oneLine(update.available.Name), width),
			fit(update.installed.Version+" → "+update.available.Version, width), "", "  Update", "  Dismiss"}
		lines[3+m.addonUpdateChoice] = highlight.Render("› " + strings.TrimSpace(lines[3+m.addonUpdateChoice]))
		return "ADDON UPDATE", strings.Join(lines, "\n"), footer
	}
	items := m.pendingAddonUpdates()
	lines := []string{}
	count := max(1, min(12, m.height-11))
	selected := min(m.menuIndex, max(0, len(items)-1))
	start := max(0, selected-count+1)
	for i := start; i < min(len(items), start+count); i++ {
		update := items[i]
		prefix := "  "
		if i == selected {
			prefix = "› "
		}
		line := fit(prefix+oneLine(update.available.Name)+" "+update.installed.Version+" → "+update.available.Version, width)
		if i == selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, muted.Render("No addon updates to review."))
	}
	return "ADDON UPDATES", strings.Join(lines, "\n"), footer
}
