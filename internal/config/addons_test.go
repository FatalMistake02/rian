package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const versionedAddonConfig = `{"version":1,"projects":[],"settings":{"addons":{"test":{"definition":{"format_version":1,"version":"1.0.0","id":"test","name":"Test","shortcut":"x","settings":[{"id":"mode","name":"Mode","type":"text","default":"old"}],"pages":[{"id":"notes","name":"Notes","text":"Hello"}]},"enabled":false,"source":"original.json","values":{"mode":"custom"}}},"hidden":{"addon-page/test/notes":true},"project_overrides":{"project":{"addon/test":false}}}}`

func TestAddonRoundTripAndUpgradePreservesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(versionedAddonConfig), 0600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	cfg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	item := cfg.Settings.Addons["test"]
	if item.Definition.FormatVersion != 1 || item.Definition.Version != "1.0.0" || item.Enabled || item.Definition.Shortcut != "x" || item.Source != "original.json" || item.Values["mode"] != "custom" {
		t.Fatalf("addon changed during roundtrip: %+v", item)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != versionedAddonConfig {
		t.Fatal("loading rewrote the configuration")
	}
	if _, err := store.Update(func(*Config) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Settings struct {
			Addons map[string]struct {
				Definition struct {
					FormatVersion int    `json:"format_version"`
					Version       string `json:"version"`
				} `json:"definition"`
			} `json:"addons"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("saved definitions do not use string release versions: %v", err)
	}
	if d := saved.Settings.Addons["test"].Definition; d.FormatVersion != 1 || d.Version != "1.0.0" {
		t.Fatalf("saved definition lost its version fields: %+v", d)
	}
	if _, err := store.Update(func(cfg *Config) error {
		item := cfg.Settings.Addons["test"]
		next := item.Definition
		next.Version = "1.1.0"
		updated, err := item.Upgrade(next)
		if err == nil {
			cfg.Settings.Addons["test"] = updated
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	item = cfg.Settings.Addons["test"]
	shown, overridden := cfg.Settings.Override("project", "addon/test")
	if item.Definition.Version != "1.1.0" || item.Enabled || item.Definition.Shortcut != "x" || item.Values["mode"] != "custom" || cfg.Settings.Shown("addon-page/test/notes") || shown || !overridden {
		t.Fatalf("upgrade lost user choices: addon=%+v settings=%+v", item, cfg.Settings)
	}
}

func TestNumericAddonVersionConfigIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := strings.Replace(versionedAddonConfig, `"format_version":1,"version":"1.0.0"`, `"format_version":1,"version":1`, 1)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path).Update(func(*Config) error { return nil }); err == nil {
		t.Fatal("numeric addon release version was accepted")
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != data {
		t.Fatal("invalid addon config was overwritten")
	}
}
