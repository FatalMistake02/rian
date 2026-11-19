package config

import (
	"path/filepath"
	"reflect"
	"rian/internal/addon"
	"testing"
)

func TestProjectSectionOrderPersistenceAndClone(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "config.json"))
	order := []string{"commands", "github", "stack", "git", "commit"}
	_, err := store.Update(func(cfg *Config) error { cfg.Settings.ProjectSectionOrder = order; return nil })
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := store.Load()
	if err != nil || !reflect.DeepEqual(cfg.Settings.ProjectSectionOrder, order) {
		t.Fatalf("round trip: %+v %v", cfg.Settings, err)
	}
	clone := cfg.Settings.Clone()
	clone.ProjectSectionOrder[0] = "stack"
	if cfg.Settings.ProjectSectionOrder[0] != "commands" {
		t.Fatal("clone shares section order")
	}
	for _, invalid := range [][]string{{"stack", "stack"}, {"unknown"}, {"addon/../escape"}} {
		s := Settings{ProjectSectionOrder: invalid}
		if err := s.validate(); err == nil {
			t.Fatalf("invalid order accepted: %v", invalid)
		}
	}
}

func TestProjectSectionsHandleAddonChanges(t *testing.T) {
	s := Settings{ProjectSectionOrder: []string{"commands", "addon/removed", "github"}, Addons: map[string]addon.Installed{
		"registry": {Definition: addon.Definition{Name: "Registry", Metadata: &addon.MetadataProvider{File: "manifest.json"}}},
		"tool":     {Definition: addon.Definition{Name: "Tool"}},
	}}
	sections := s.ProjectSections()
	want := []string{"commands", "github", "stack", "addon/registry", "git", "commit"}
	got := []string{}
	for _, section := range sections {
		got = append(got, section.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("section order: got %v want %v", got, want)
	}
}
