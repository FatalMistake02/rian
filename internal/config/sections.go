package config

import (
	"regexp"
	"rian/internal/addon"
)

var addonSectionID = regexp.MustCompile(`^addon/[a-z][a-z0-9-]{0,63}$`)

type ProjectSection struct{ ID, Name string }

// ProjectSections keeps known saved positions and appends newly available sections.
func (s Settings) ProjectSections() []ProjectSection {
	defaults := []ProjectSection{{"stack", "Tech stack"}}
	for _, id := range addon.IDs(s.Addons) {
		item := s.Addons[id]
		if item.Definition.Metadata != nil {
			defaults = append(defaults, ProjectSection{"addon/" + id, item.Definition.Name})
		}
	}
	defaults = append(defaults, ProjectSection{"github", "GitHub"}, ProjectSection{"git", "Git status"}, ProjectSection{"commit", "Last commit"}, ProjectSection{"commands", "Commands"})
	available := map[string]ProjectSection{}
	for _, section := range defaults {
		available[section.ID] = section
	}
	ordered := []ProjectSection{}
	for _, id := range s.ProjectSectionOrder {
		if section, ok := available[id]; ok {
			ordered = append(ordered, section)
			delete(available, id)
		}
	}
	for _, section := range defaults {
		if _, ok := available[section.ID]; ok {
			ordered = append(ordered, section)
			delete(available, section.ID)
		}
	}
	return ordered
}
