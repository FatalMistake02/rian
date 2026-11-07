package config

import (
	"fmt"
	"rian/internal/addon"
)

type Settings struct {
	AutoAddEnabled     bool                       `json:"auto_add_enabled,omitempty"`
	AutoAddLocation    string                     `json:"auto_add_location,omitempty"`
	AutoAddHidden      bool                       `json:"auto_add_hidden,omitempty"`
	Addons             map[string]addon.Installed `json:"addons,omitempty"`
	GraphScope         string                     `json:"git_graph_scope,omitempty"`
	ProjectOpening     string                     `json:"project_opening,omitempty"`
	DefaultPage        string                     `json:"default_page,omitempty"`
	OpenProjectOnStart *bool                      `json:"open_project_on_start,omitempty"`
	Overrides          map[string]map[string]bool `json:"project_overrides,omitempty"`
	Accent             string                     `json:"accent,omitempty"`
	Hidden             map[string]bool            `json:"hidden,omitempty"`
	Launch             map[string]string          `json:"launch,omitempty"`
}

func (s Settings) Shown(id string) bool { return !s.Hidden[id] }
func (s Settings) LaunchMode(id string) string {
	if mode := s.Launch[id]; mode != "" && mode != "tab" {
		return mode
	}
	return "window"
}
func CommandKey(projectID, name string) string { return "command/" + projectID + "/" + name }
func (s Settings) Clone() Settings {
	copy := Settings{AutoAddEnabled: s.AutoAddEnabled, AutoAddLocation: s.AutoAddLocation, AutoAddHidden: s.AutoAddHidden, GraphScope: s.GraphScope, ProjectOpening: s.ProjectOpening, DefaultPage: s.DefaultPage, Accent: s.Accent, Overrides: map[string]map[string]bool{}, Hidden: map[string]bool{}, Launch: map[string]string{}}
	if s.OpenProjectOnStart != nil {
		value := *s.OpenProjectOnStart
		copy.OpenProjectOnStart = &value
	}
	for id, values := range s.Overrides {
		copy.Overrides[id] = map[string]bool{}
		for key, value := range values {
			copy.Overrides[id][key] = value
		}
	}
	for k, v := range s.Hidden {
		copy.Hidden[k] = v
	}
	for k, v := range s.Launch {
		if v == "tab" {
			v = "window"
		}
		copy.Launch[k] = v
	}
	copy.Addons = map[string]addon.Installed{}
	for id, item := range s.Addons {
		copy.Addons[id] = item.Clone()
	}
	return copy
}
func (s Settings) validate() error {
	for id, item := range s.Addons {
		if id != item.Definition.ID {
			return fmt.Errorf("add-on id mismatch")
		}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("add-on %s: %w", id, err)
		}
	}
	if s.GraphScope != "" && s.GraphScope != "current" && s.GraphScope != "all" {
		return fmt.Errorf("invalid Git graph scope %q", s.GraphScope)
	}
	if s.ProjectOpening != "" && s.ProjectOpening != "project" && s.ProjectOpening != "normal" {
		return fmt.Errorf("invalid opening behavior inside a project %q", s.ProjectOpening)
	}
	if s.DefaultPage != "" && s.DefaultPage != "overview" && s.DefaultPage != "projects" && s.DefaultPage != "current-project" {
		return fmt.Errorf("invalid default opening page %q", s.DefaultPage)
	}
	if s.Accent != "" {
		found := false
		for _, color := range AccentColors {
			if color.ID == s.Accent {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown accent color %q", s.Accent)
		}
	}
	for id, mode := range s.Launch {
		if mode != "same" && mode != "window" && mode != "ask" {
			return fmt.Errorf("invalid launch mode %q for %s", mode, id)
		}
	}
	return nil
}

type AccentColor struct{ ID, Name, Hex string }

var AccentColors = []AccentColor{
	{"fatal-pink", "fatal pink", "#fa11b0"},
	{"glacial-blue", "glacial blue", "#8ccfe8"},
	{"mint-green", "mint green", "#98f5c9"},
	{"amber", "amber", "#f5b942"},
	{"violet", "violet", "#b58aff"},
	{"coral", "coral", "#ff806c"},
	{"random", "random", "#fa11b0"},
}

func (s Settings) AccentColor() AccentColor {
	id := s.Accent
	if id == "electric-blue" {
		id = "glacial-blue"
	}
	for _, color := range AccentColors {
		if color.ID == id {
			return color
		}
	}
	return AccentColors[0]
}

func (s Settings) Override(projectID, id string) (bool, bool) {
	value, set := s.Overrides[projectID][id]
	return value, set
}
func (s Settings) ShownFor(projectID, id string) bool {
	if value, set := s.Override(projectID, id); set {
		return value
	}
	return s.Shown(id)
}

func (s Settings) StartPage() string {
	if s.DefaultPage == "" || s.DefaultPage == "current-project" {
		return "overview"
	}
	return s.DefaultPage
}
func (s Settings) ProjectLaunchMode() string {
	if s.ProjectOpening != "" {
		return s.ProjectOpening
	}
	if s.OpenProjectOnStart != nil && !*s.OpenProjectOnStart {
		return "normal"
	}
	return "project"
}
func (s Settings) AutoOpenProject() bool { return s.ProjectLaunchMode() == "project" }

func (s Settings) GitGraphScope() string {
	if s.GraphScope == "all" {
		return "all"
	}
	return "current"
}
