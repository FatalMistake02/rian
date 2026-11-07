// Package addon implements declarative integrations. Detection only inspects
// files and PATH; commands are prepared only after a user selects an action.
package addon

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"rian/internal/project"
	"rian/internal/tools"
)

type Detection struct {
	Files []string `json:"files,omitempty"`
	Tools []string `json:"tools,omitempty"`
}
type Setting struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Default any      `json:"default"`
	Options []string `json:"options,omitempty"`
}
type Action struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Executable string    `json:"executable,omitempty"`
	Args       []string  `json:"args,omitempty"`
	URL        string    `json:"url,omitempty"`
	Detect     Detection `json:"detect,omitempty"`
}
type PageSection struct {
	Title   string   `json:"title,omitempty"`
	Text    string   `json:"text,omitempty"`
	Actions []string `json:"actions,omitempty"`
}
type Page struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Title    string        `json:"title,omitempty"`
	Text     string        `json:"text,omitempty"`
	Detect   Detection     `json:"detect,omitempty"`
	Actions  []string      `json:"actions,omitempty"`
	Sections []PageSection `json:"sections,omitempty"`
}
type Definition struct {
	Pages         []Page    `json:"pages,omitempty"`
	Menu          bool      `json:"menu,omitempty"`
	FormatVersion int       `json:"format_version"`
	Version       string    `json:"version"`
	UpdateURL     string    `json:"update_url,omitempty"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Shortcut      string    `json:"shortcut,omitempty"`
	Detect        Detection `json:"detect,omitempty"`
	Settings      []Setting `json:"settings,omitempty"`
	Actions       []Action  `json:"actions"`
}
type Installed struct {
	Definition Definition        `json:"definition"`
	Enabled    bool              `json:"enabled"`
	Source     string            `json:"source,omitempty"`
	Values     map[string]string `json:"values,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func Read(path string) (Installed, error) {
	file, err := os.Open(path)
	if err != nil {
		return Installed{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestSize+1))
	if err != nil {
		return Installed{}, err
	}
	d, err := Parse(data)
	if err != nil {
		return Installed{}, err
	}
	path, _ = filepath.Abs(path)
	return Installed{Definition: d, Enabled: true, Source: path, Values: map[string]string{}}, nil
}
func (d Detection) Validate() error {
	for _, pattern := range d.Files {
		if pattern == "" || filepath.IsAbs(pattern) || strings.Contains(filepath.ToSlash(pattern), "..") {
			return fmt.Errorf("file detection must use relative project paths")
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return err
		}
	}
	for _, tool := range d.Tools {
		if tool == "" || strings.ContainsAny(tool, "/\\\x00") {
			return fmt.Errorf("tool detection requires an executable name")
		}
	}
	return nil
}
func (d Detection) Matches(root string) bool {
	for _, tool := range d.Tools {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}
	if len(d.Files) == 0 {
		return true
	}
	for _, pattern := range d.Files {
		matches, _ := filepath.Glob(filepath.Join(root, pattern))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}
func (d Definition) Validate() error {
	if d.FormatVersion != 1 {
		return fmt.Errorf("unsupported add-on format version %d", d.FormatVersion)
	}
	if _, ok := parseVersion(d.Version); !ok {
		return fmt.Errorf("add-on version must be a semantic version such as 1.0.0")
	}
	if err := tools.ValidateURL(d.UpdateURL); err != nil {
		return fmt.Errorf("invalid add-on update URL: %w", err)
	}
	if !identifier.MatchString(d.ID) || strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("add-on needs a lowercase id and a name")
	}
	if len(d.Actions) == 0 && len(d.Pages) == 0 || len(d.Actions) > 100 || len(d.Settings) > 100 {
		return fmt.Errorf("add-on needs actions or pages, and at most 100 actions and settings")
	}
	if err := d.Detect.Validate(); err != nil {
		return err
	}
	if !ValidShortcut(d.Shortcut) {
		return fmt.Errorf("shortcut must be one letter or empty")
	}
	ids := map[string]bool{}
	for _, s := range d.Settings {
		if !identifier.MatchString(s.ID) || s.Name == "" || ids[s.ID] {
			return fmt.Errorf("invalid or duplicate setting %q", s.ID)
		}
		ids[s.ID] = true
		switch s.Default.(type) {
		case nil, string, bool, float64, int:
		default:
			return fmt.Errorf("setting %s requires a scalar default", s.ID)
		}
		if s.Type == "choice" && len(s.Options) == 0 {
			return fmt.Errorf("setting %s needs choices", s.ID)
		}
		if err := s.ValidateValue(s.DefaultValue()); err != nil {
			return err
		}
	}
	ids = map[string]bool{}
	for _, a := range d.Actions {
		if !identifier.MatchString(a.ID) || a.Name == "" || ids[a.ID] {
			return fmt.Errorf("invalid or duplicate action %q", a.ID)
		}
		ids[a.ID] = true
		if err := a.Detect.Validate(); err != nil {
			return err
		}
		switch a.Kind {
		case "url":
			if a.URL == "" || a.Executable != "" || len(a.Args) > 0 {
				return fmt.Errorf("URL action requires only url")
			}
		case "cli", "command", "app", "output":
			if a.Executable == "" || strings.ContainsAny(a.Executable, "/\\\x00$") || a.URL != "" {
				return fmt.Errorf("action %s requires an executable name", a.ID)
			}
		default:
			return fmt.Errorf("unknown action kind %q", a.Kind)
		}
		for _, v := range append(append([]string{}, a.Args...), a.URL) {
			for _, match := range regexp.MustCompile(`\$\{([^}]+)\}`).FindAllStringSubmatch(v, -1) {
				if match[1] != "project.path" && match[1] != "project.name" {
					id := strings.TrimPrefix(match[1], "setting.")
					found := false
					for _, s := range d.Settings {
						found = found || strings.HasPrefix(match[1], "setting.") && s.ID == id
					}
					if !found {
						return fmt.Errorf("unknown template %s", match[0])
					}
				}
			}
		}
	}
	pageIDs := map[string]bool{}
	if len(d.Pages) > 50 {
		return fmt.Errorf("at most 50 addon pages are supported")
	}
	for _, page := range d.Pages {
		if !identifier.MatchString(page.ID) || strings.TrimSpace(page.Name) == "" || pageIDs[page.ID] {
			return fmt.Errorf("invalid or duplicate page %q", page.ID)
		}
		pageIDs[page.ID] = true
		if err := page.Detect.Validate(); err != nil {
			return err
		}
		refs := append([]string{}, page.Actions...)
		for _, section := range page.Sections {
			refs = append(refs, section.Actions...)
		}
		for _, id := range refs {
			if !ids[id] {
				return fmt.Errorf("page %s references unknown action %s", page.ID, id)
			}
		}
	}
	return nil
}
func ValidShortcut(s string) bool {
	return s == "" || len(s) == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}
func (s Setting) DefaultValue() string {
	if s.Default == nil {
		return ""
	}
	return fmt.Sprint(s.Default)
}
func (s Setting) ValidateValue(value string) error {
	switch s.Type {
	case "text":
		return nil
	case "bool":
		if value == "true" || value == "false" {
			return nil
		}
	case "number":
		if number, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			return nil
		}
	case "choice":
		for _, v := range s.Options {
			if v == value {
				return nil
			}
		}
	default:
		return fmt.Errorf("unknown setting type %q", s.Type)
	}
	return fmt.Errorf("invalid value for %s", s.Name)
}
func (i Installed) Value(s Setting) string {
	if v, ok := i.Values[s.ID]; ok {
		return v
	}
	return s.DefaultValue()
}
func (i Installed) Validate() error {
	if err := i.Definition.Validate(); err != nil {
		return err
	}
	for id, v := range i.Values {
		found := false
		for _, s := range i.Definition.Settings {
			if s.ID == id {
				found = true
				if err := s.ValidateValue(v); err != nil {
					return err
				}
			}
		}
		if !found {
			return fmt.Errorf("unknown setting %s", id)
		}
	}
	return nil
}
func (i Installed) Clone() Installed {
	data, _ := json.Marshal(i)
	var c Installed
	_ = json.Unmarshal(data, &c)
	return c
}
func IDs(items map[string]Installed) []string {
	ids := []string{}
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		return strings.ToLower(items[ids[a]].Definition.Name) < strings.ToLower(items[ids[b]].Definition.Name)
	})
	return ids
}
func (i Installed) Actions(p project.Project) []Action {
	out := []Action{}
	if !i.Enabled || !i.Definition.Detect.Matches(p.Path) {
		return out
	}
	info, err := os.Stat(p.Path)
	directoryAvailable := err == nil && info.IsDir()
	for _, a := range i.Definition.Actions {
		if a.Kind != "url" && !directoryAvailable {
			continue
		}
		if !a.Detect.Matches(p.Path) {
			continue
		}
		if a.Executable != "" {
			if _, err := exec.LookPath(a.Executable); err != nil {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}
func (i Installed) Expand(value string, p project.Project) string {
	replacements := []string{"${project.path}", p.Path, "${project.name}", p.Name}
	for _, s := range i.Definition.Settings {
		replacements = append(replacements, "${setting."+s.ID+"}", i.Value(s))
	}
	return strings.NewReplacer(replacements...).Replace(value)
}

func (i Installed) Command(a Action, p project.Project) (*exec.Cmd, error) {
	args := make([]string, len(a.Args))
	for j, v := range a.Args {
		args[j] = i.Expand(v, p)
	}
	return tools.ExternalCommand(p, a.Executable, args...)
}
