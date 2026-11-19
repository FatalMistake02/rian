package ui

import (
	"fmt"
	"rian/internal/detect"
	"rian/internal/project"
	"strings"
)

func (m *Model) projectDetailSections(p project.Project, width int) []string {
	s := m.statuses[p.ID]
	repo := p.GitHub
	if repo == "" {
		repo = s.GitHub
	}
	label := func(name, value string) string {
		return muted.Render(pad(name, min(14, width/3))) + textStyle.Render(fit(oneLine(value), max(1, width-min(14, width/3))))
	}
	lines := []string{}
	for _, section := range m.cfg.Settings.ProjectSections() {
		switch section.ID {
		case "stack":
			if len(p.Stack) > 0 {
				lines = append(lines, muted.Render(fit(strings.Join(p.Stack, " · "), width)), "")
			}
		case "github":
			if repo != "" {
				lines = append(lines, highlight.Render("GITHUB"), textStyle.Render(fit(oneLine(repo), width)), "")
			}
		case "git":
			if s.Repository {
				state := good.Render("○ clean")
				if s.Dirty() {
					state = warning.Render("● changes")
				}
				lines = append(lines, label("Branch", s.Branch), label("Status", "")+state, label("Modified", fmt.Sprint(s.Modified)), label("Untracked", fmt.Sprint(s.Untracked)), label("Ahead / back", fmt.Sprintf("↑ %d   ↓ %d", s.Ahead, s.Behind)), "")
				if s.Remote != "" && repo == "" {
					lines = append(lines, label("Remote", s.Remote))
				}
			} else if m.loading[p.ID] {
				lines = append(lines, m.spin.View()+muted.Render(" Reading Git status"), "")
			} else {
				lines = append(lines, muted.Render("No Git repository detected."), "")
			}
			if s.Error != "" {
				lines = append(lines, fit(warning.Render(s.Error), width))
			}
		case "commit":
			if s.Repository && s.Commit != "" {
				lines = append(lines, muted.Render("LAST COMMIT"), fit(oneLine(s.Commit), width), muted.Render(relativeTime(s.CommitTime)), "")
			}
		case "commands":
			if m.projectFull {
				commands := m.visibleCommands(p)
				if len(commands) > 0 {
					lines = append(lines, highlight.Render("COMMANDS"))
					for _, name := range detect.CommandNames(commands) {
						lines = append(lines, fit(textStyle.Render(name)+muted.Render("  "+oneLine(commands[name])), width))
					}
					lines = append(lines, "")
				}
			}
		default:
			if strings.HasPrefix(section.ID, "addon/") {
				lines = append(lines, m.addonMetadataDetail(strings.TrimPrefix(section.ID, "addon/"), p, width)...)
			}
		}
	}
	return lines
}

// moveProjectSection applies a live settings preview and keeps the moved row selected.
func (m *Model) moveProjectSection(id string, step int) {
	sections := m.settingsDraft.ProjectSections()
	for i, section := range sections {
		if section.ID != id {
			continue
		}
		target := i + step
		if target < 0 || target >= len(sections) {
			return
		}
		sections[i], sections[target] = sections[target], sections[i]
		m.settingsDraft.ProjectSectionOrder = make([]string, len(sections))
		for j, entry := range sections {
			m.settingsDraft.ProjectSectionOrder[j] = entry.ID
		}
		m.applySettingsChange()
		m.buildSettingsRows()
		for j, row := range m.settingsMatches() {
			if row.operation == "section-order" && row.id == id {
				m.settingsIndex = j
				break
			}
		}
		return
	}
}
