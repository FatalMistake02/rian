package addon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rian/internal/project"
	"rian/internal/tools"
)

// MetadataProvider combines a local JSON file and an optional JSON GET.
type MetadataProvider struct {
	File          string              `json:"file,omitempty"`
	Fetch         string              `json:"fetch,omitempty"`
	FetchWhen     []MetadataCondition `json:"fetch_when,omitempty"`
	HiddenWhen    []MetadataCondition `json:"hidden_when,omitempty"`
	VisibleWhen   []MetadataCondition `json:"visible_when,omitempty"`
	FilterSetting string              `json:"filter_setting,omitempty"`
	URLWhen       []MetadataCondition `json:"url_when,omitempty"`
	Title         string              `json:"title,omitempty"`
	Name          string              `json:"name,omitempty"`
	Summary       string              `json:"summary,omitempty"`
	Status        string              `json:"status,omitempty"`
	Statuses      []MetadataStatus    `json:"statuses,omitempty"`
	URL           string              `json:"url,omitempty"`
	Fields        []MetadataField     `json:"fields,omitempty"`
}
type MetadataCondition struct {
	Path   string   `json:"path"`
	Op     string   `json:"op"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}
type MetadataStatus struct {
	When []MetadataCondition `json:"when"`
	Text string              `json:"text"`
}
type MetadataField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Metadata struct {
	Title, Name, Summary, Status, URL string
	Fields                            []MetadataField
	Hidden                            bool
}

var metadataTemplate = regexp.MustCompile(`\$\{([^}]+)\}`)

func metadataPath(path string) bool {
	path = strings.TrimSuffix(path, "|path")
	return strings.HasPrefix(path, "file.") && len(path) > 5 || strings.HasPrefix(path, "remote.") && len(path) > 7 || path == "request.status" || path == "request.error" || path == "project.name" || path == "status"
}
func (p MetadataProvider) Validate(settings []Setting) error {
	if p.FilterSetting != "" {
		found := false
		for _, setting := range settings {
			found = found || setting.ID == p.FilterSetting && setting.Type == "bool"
		}
		if !found {
			return fmt.Errorf("metadata filter_setting requires an existing boolean setting")
		}
	}
	if p.File == "" && p.Fetch == "" {
		return fmt.Errorf("metadata requires a file or fetch URL")
	}
	if filepath.IsAbs(p.File) || strings.ContainsAny(p.File, "\x00$:") || strings.Contains(strings.ReplaceAll(p.File, "\\", "/"), "..") {
		return fmt.Errorf("metadata file must be a relative project path")
	}
	if len(p.Fields) > 20 || len(p.Statuses) > 20 {
		return fmt.Errorf("metadata supports at most 20 fields and status rules")
	}
	conditions := append(append(append([]MetadataCondition{}, p.FetchWhen...), p.HiddenWhen...), p.URLWhen...)
	conditions = append(conditions, p.VisibleWhen...)
	texts := []string{p.Fetch, p.Title, p.Name, p.Summary, p.Status, p.URL}
	for _, field := range p.Fields {
		texts = append(texts, field.Label, field.Value)
	}
	for _, rule := range p.Statuses {
		conditions = append(conditions, rule.When...)
		texts = append(texts, rule.Text)
	}
	if len(conditions) > 100 {
		return fmt.Errorf("at most 100 metadata conditions are supported")
	}
	for _, c := range conditions {
		if !metadataPath(c.Path) {
			return fmt.Errorf("unknown metadata path %q", c.Path)
		}
		switch c.Op {
		case "empty", "present", "eq", "ne", "in":
		default:
			return fmt.Errorf("unknown metadata operation %q", c.Op)
		}
		texts = append(texts, c.Value)
	}
	for _, text := range texts {
		for _, match := range metadataTemplate.FindAllStringSubmatch(text, -1) {
			if !metadataPath(match[1]) {
				return fmt.Errorf("unknown metadata template %s", match[0])
			}
		}
	}
	for _, address := range []string{p.Fetch, p.URL} {
		if address == "" {
			continue
		}
		// Parse a substituted URL, then verify its origin is entirely literal.
		parsed, err := url.Parse(metadataTemplate.ReplaceAllString(address, "value"))
		if err != nil {
			return fmt.Errorf("invalid metadata URL")
		}
		if tools.ValidateURL(parsed.String()) != nil {
			return fmt.Errorf("metadata URLs require HTTP(S)")
		}
		parts := strings.SplitN(address, "://", 2)
		if len(parts) != 2 || strings.Contains(parts[0], "${") || strings.Contains(strings.SplitN(parts[1], "/", 2)[0], "${") || parsed.User != nil {
			return fmt.Errorf("metadata URLs require a fixed host without credentials")
		}
	}
	return nil
}

type metadataValues map[string]any

func (v metadataValues) get(path string) string {
	var current any = map[string]any(v)
	for _, part := range strings.Split(path, ".") {
		switch object := current.(type) {
		case map[string]any:
			current = object[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(object) {
				return ""
			}
			current = object[index]
		default:
			return ""
		}
	}
	switch value := current.(type) {
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case json.Number:
		return value.String()
	}
	return ""
}
func (v metadataValues) expand(text string, encode bool) string {
	return metadataTemplate.ReplaceAllStringFunc(text, func(token string) string {
		key := token[2 : len(token)-1]
		pathValue := strings.HasSuffix(key, "|path")
		value := v.get(strings.TrimSuffix(key, "|path"))
		if encode {
			if pathValue {
				parts := strings.Split(value, "/")
				for j := range parts {
					parts[j] = url.PathEscape(parts[j])
				}
				return strings.Join(parts, "/")
			}
			return url.PathEscape(value)
		}
		return value
	})
}
func (v metadataValues) matches(conditions []MetadataCondition) bool {
	for _, c := range conditions {
		value, want := v.get(c.Path), v.expand(c.Value, false)
		match := false
		switch c.Op {
		case "empty":
			match = value == ""
		case "present":
			match = value != ""
		case "eq":
			match = value == want
		case "ne":
			match = value != want
		case "in":
			for _, candidate := range c.Values {
				match = match || value == candidate
			}
		}
		if !match {
			return false
		}
	}
	return true
}
func readMetadataJSON(reader io.Reader) (any, error) {
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("JSON exceeds 1 MiB")
	}
	var object any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&object); err != nil {
		return nil, err
	}
	var extra any
	switch object.(type) {
	case map[string]any, []any:
	default:
		return nil, fmt.Errorf("expected a JSON object or array")
	}
	if dec.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("expected one JSON value")
	}
	return object, nil
}

func (i Installed) LoadMetadata(ctx context.Context, p project.Project) (Metadata, error) {
	return i.LoadMetadataWithClient(ctx, p, http.DefaultClient)
}
func (i Installed) LoadMetadataWithClient(ctx context.Context, p project.Project, client *http.Client) (Metadata, error) {
	provider := i.Definition.Metadata
	if !i.Enabled || provider == nil || !i.Definition.Detect.Matches(p.Path) {
		return Metadata{}, fmt.Errorf("metadata unavailable")
	}
	var local any = map[string]any{}
	if provider.File != "" {
		f, err := os.Open(filepath.Join(p.Path, provider.File))
		if err != nil {
			return Metadata{}, err
		}
		local, err = readMetadataJSON(f)
		f.Close()
		if err != nil {
			return Metadata{}, fmt.Errorf("invalid local JSON: %w", err)
		}
	}
	values := metadataValues{"file": local, "remote": map[string]any{}, "request": map[string]any{}, "project": map[string]any{"name": p.Name}}
	if len(provider.HiddenWhen) > 0 && values.matches(provider.HiddenWhen) {
		return Metadata{Hidden: true}, nil
	}
	request := values["request"].(map[string]any)
	if provider.Fetch != "" && values.matches(provider.FetchWhen) {
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, values.expand(provider.Fetch, true), nil)
		if err != nil {
			return Metadata{}, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "rian-addon-metadata")
		resp, err := client.Do(req)
		if err != nil {
			request["error"] = "Request failed"
		} else {
			defer resp.Body.Close()
			request["status"] = strconv.Itoa(resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				remote, err := readMetadataJSON(resp.Body)
				if err != nil {
					request["error"] = "Invalid JSON response"
				} else {
					values["remote"] = remote
				}
			} else if resp.StatusCode != http.StatusNotFound {
				request["error"] = "HTTP " + strconv.Itoa(resp.StatusCode)
			}
		}
	}
	if i.FilterMetadata() && !values.matches(provider.VisibleWhen) {
		return Metadata{Hidden: true}, nil
	}
	status := values.expand(provider.Status, false)
	for _, rule := range provider.Statuses {
		if values.matches(rule.When) {
			status = values.expand(rule.Text, false)
			break
		}
	}
	values["status"] = status
	result := Metadata{Title: values.expand(provider.Title, false), Name: values.expand(provider.Name, false), Summary: values.expand(provider.Summary, false), Status: status}
	if values.matches(provider.URLWhen) {
		result.URL = values.expand(provider.URL, true)
		if err := tools.ValidateURL(result.URL); err != nil {
			return Metadata{}, fmt.Errorf("invalid metadata link: %w", err)
		}
	}
	for _, field := range provider.Fields {
		value := values.expand(field.Value, false)
		if value != "" {
			result.Fields = append(result.Fields, MetadataField{values.expand(field.Label, false), value})
		}
	}
	return result, nil
}

// FilterMetadata reports whether the addon's optional visibility filter is on.
func (i Installed) FilterMetadata() bool {
	p := i.Definition.Metadata
	if p == nil || len(p.VisibleWhen) == 0 {
		return false
	}
	if p.FilterSetting == "" {
		return true
	}
	for _, setting := range i.Definition.Settings {
		if setting.ID == p.FilterSetting {
			return i.Value(setting) == "true"
		}
	}
	return false
}
