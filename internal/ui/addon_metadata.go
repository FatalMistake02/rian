package ui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"rian/internal/addon"
	"rian/internal/project"
)

type addonMetadataState struct {
	data        addon.Metadata
	fingerprint string
	checked     time.Time
	loading     bool
	err         error
}
type addonMetadataMsg struct {
	key, fingerprint string
	data             addon.Metadata
	err              error
}

func metadataKey(id string, p project.Project) string { return id + "/" + p.ID }
func metadataFingerprint(item addon.Installed, p project.Project) string {
	data, _ := json.Marshal(struct {
		Item addon.Installed
		Path string
	}{item, p.Path})
	return string(data)
}
func (m *Model) metadataEnabled(id string, item addon.Installed, p project.Project) bool {
	return item.Enabled && item.Definition.Metadata != nil && m.shownFor(p, "addon/"+id) && item.Definition.Detect.Matches(p.Path)
}
func (m *Model) refreshAddonMetadata() tea.Cmd {
	if m.addonMetadata == nil {
		m.addonMetadata = map[string]addonMetadataState{}
	}
	wanted := map[string]string{}
	for id, item := range m.cfg.Settings.Addons {
		for _, p := range m.cfg.Projects {
			if m.metadataEnabled(id, item, p) {
				wanted[metadataKey(id, p)] = metadataFingerprint(item, p)
			}
		}
	}
	count := 0
	for key, state := range m.addonMetadata {
		if wanted[key] != state.fingerprint {
			delete(m.addonMetadata, key)
			continue
		}
		if state.loading {
			count++
		}
	}
	cmds := []tea.Cmd{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		item := m.cfg.Settings.Addons[id]
		for _, p := range m.cfg.Projects {
			if count >= 4 {
				return tea.Batch(cmds...)
			}
			key := metadataKey(id, p)
			fingerprint, ok := wanted[key]
			if !ok {
				continue
			}
			state := m.addonMetadata[key]
			if state.loading || time.Since(state.checked) < 5*time.Minute {
				continue
			}
			state.loading, state.fingerprint = true, fingerprint
			m.addonMetadata[key] = state
			count++
			cmds = append(cmds, func() tea.Msg {
				data, err := item.LoadMetadata(context.Background(), p)
				return addonMetadataMsg{key, fingerprint, data, err}
			})
		}
	}
	return tea.Batch(cmds...)
}
func (m *Model) metadataFor(id string, p project.Project) (addon.Metadata, string) {
	state, ok := m.addonMetadata[metadataKey(id, p)]
	if !ok || state.fingerprint != metadataFingerprint(m.cfg.Settings.Addons[id], p) || state.checked.IsZero() {
		return addon.Metadata{Name: p.Name}, "Loading addon data…"
	}
	if state.err != nil {
		return addon.Metadata{Name: p.Name}, "Addon data unavailable"
	}
	return state.data, ""
}
func (m *Model) invalidateAddonMetadata(projectID string) {
	for key, state := range m.addonMetadata {
		if projectID == "" || strings.HasSuffix(key, "/"+projectID) {
			state.checked = time.Time{}
			m.addonMetadata[key] = state
		}
	}
}
func (m *Model) addonMetadataDetails(p project.Project, width int) []string {
	lines := []string{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		item := m.cfg.Settings.Addons[id]
		if !m.metadataEnabled(id, item, p) {
			continue
		}
		data, status := m.metadataFor(id, p)
		if data.Hidden || status != "" && item.FilterMetadata() {
			continue
		}
		title := data.Title
		if title == "" {
			title = item.Definition.Name
		}
		lines = append(lines, highlight.Render(fit(oneLine(title), width)))
		if data.Name != "" {
			lines = append(lines, fit(oneLine(data.Name), width))
		}
		if status == "" {
			status = data.Status
		}
		for _, field := range data.Fields {
			lines = append(lines, muted.Render(fit(oneLine(field.Label)+" · "+oneLine(field.Value), width)))
		}
		if status != "" {
			lines = append(lines, muted.Render(fit(oneLine(status), width)))
		}
		lines = append(lines, "")
	}
	return lines
}
