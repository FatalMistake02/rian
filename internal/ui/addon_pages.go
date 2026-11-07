package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"rian/internal/addon"
	"rian/internal/project"
)

type addonOverviewPage struct {
	addonID string
	item    addon.Installed
	page    addon.Page
}
type addonPageAction struct {
	project project.Project
	action  Action
	section string
}

func (m *Model) addonOverviewPages() []addonOverviewPage {
	pages := []addonOverviewPage{}
	for _, id := range addon.IDs(m.cfg.Settings.Addons) {
		item := m.cfg.Settings.Addons[id]
		if !item.Enabled {
			continue
		}
		for _, page := range item.Definition.Pages {
			if m.cfg.Settings.Shown("addon-page/" + id + "/" + page.ID) {
				pages = append(pages, addonOverviewPage{id, item, page})
			}
		}
	}
	return pages
}
func (m *Model) overviewPageNames() []string {
	names := []string{"Projects", "Settings"}
	for _, page := range m.addonOverviewPages() {
		names = append(names, oneLine(page.page.Name))
	}
	return names
}
func (m *Model) selectedAddonPage() (addonOverviewPage, bool) {
	pages := m.addonOverviewPages()
	idx := m.overviewIndex - 2
	if idx < 0 || idx >= len(pages) {
		return addonOverviewPage{}, false
	}
	return pages[idx], true
}
func (m *Model) addonPageActions(page addonOverviewPage) []addonPageAction {
	rows := []addonPageAction{}
	sections := page.page.Sections
	if len(sections) == 0 {
		sections = []addon.PageSection{{Actions: page.page.Actions}}
	}
	for _, section := range sections {
		for _, p := range m.cfg.Projects {
			if !m.shownFor(p, "addon/"+page.addonID) || !page.page.Detect.Matches(p.Path) {
				continue
			}
			available := page.item.Actions(p)
			// An omitted action list uses all available actions in manifest order.
			refs := section.Actions
			if len(refs) == 0 {
				for _, a := range available {
					refs = append(refs, a.ID)
				}
			}
			for _, id := range refs {
				for _, a := range available {
					if a.ID == id {
						rows = append(rows, addonPageAction{p, Action{ID: "addon-action/" + page.addonID + "/" + a.ID, Label: a.Name}, section.Title})
					}
				}
			}
		}
	}
	return rows
}
func (m *Model) addonPageKey(key string) (bool, tea.Cmd) {
	page, ok := m.selectedAddonPage()
	if !ok {
		m.overviewFocused = false
		return true, nil
	}
	rows := m.addonPageActions(page)
	m.overviewAlertIndex = min(m.overviewAlertIndex, max(0, len(rows)-1))
	switch key {
	case "left", "esc", "tab", "shift+tab":
		m.overviewFocused = false
	case "up", "j":
		m.overviewAlertIndex = max(0, m.overviewAlertIndex-1)
	case "down", "k":
		m.overviewAlertIndex = min(max(0, len(rows)-1), m.overviewAlertIndex+1)
	case "home":
		m.overviewAlertIndex = 0
	case "end":
		m.overviewAlertIndex = max(0, len(rows)-1)
	case "enter", "space", " ":
		if len(rows) > 0 {
			row := rows[m.overviewAlertIndex]
			m.search.Reset()
			m.selectID(row.project.ID)
			return true, m.executeAddon(row.action, row.project)
		}
	case "a", "+", "s", "?", "q", "ctrl+k":
		return false, nil
	}
	return true, nil
}
func (m *Model) addonPageView(page addonOverviewPage, width, height int) string {
	title := page.page.Title
	if title == "" {
		title = page.page.Name
	}
	lines := []string{titleStyle.Render(oneLine(title)), ""}
	expand := func(value string) string { return plain(page.item.Expand(value, project.Project{})) }
	if page.page.Text != "" {
		lines = append(lines, strings.Split(expand(page.page.Text), "\n")...)
		lines = append(lines, "")
	}
	for _, section := range page.page.Sections {
		if section.Title != "" {
			lines = append(lines, titleStyle.Render(oneLine(section.Title)))
		}
		if section.Text != "" {
			lines = append(lines, strings.Split(expand(section.Text), "\n")...)
		}
		if section.Title != "" || section.Text != "" {
			lines = append(lines, "")
		}
	}
	rows := m.addonPageActions(page)
	if len(rows) == 0 && len(page.item.Definition.Actions) > 0 {
		lines = append(lines, muted.Render("No available actions for your projects."))
	} else if len(rows) > 0 {
		// Descriptions must leave room for keyboard selection. Reserve up to
		// half the page for actions, and mark any clipped introductory text.
		actionHeight := min(len(rows), max(1, height/2))
		introHeight := max(0, height-actionHeight)
		if len(lines) > introHeight {
			lines = lines[:introHeight]
			if introHeight > 1 {
				lines[introHeight-1] = muted.Render("…")
			}
		}
		count := max(1, height-len(lines))
		selected := min(m.overviewAlertIndex, len(rows)-1)
		start := max(0, selected-count+1)
		for idx := start; idx < min(len(rows), start+count); idx++ {
			row := rows[idx]
			prefix := "  "
			if m.overviewFocused && idx == selected {
				prefix = "› "
			}
			label := prefix + oneLine(row.project.Name) + " / " + oneLine(row.action.Label)
			if row.section != "" {
				label = prefix + oneLine(row.section) + " / " + oneLine(row.project.Name) + " / " + oneLine(row.action.Label)
			}
			line := pad(label, width)
			if m.overviewFocused && idx == selected {
				line = selectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}
	for idx := range lines {
		lines[idx] = fit(lines[idx], width)
	}
	return strings.Join(lines[:min(len(lines), max(1, height))], "\n")
}
