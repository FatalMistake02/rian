package addon

import (
	"maps"
	"testing"
)

func versionedDefinition(value string) Definition {
	return Definition{FormatVersion: 1, Version: value, ID: "test", Name: "Test", Actions: []Action{{ID: "open", Name: "Open", Kind: "url", URL: "https://example.com"}}}
}

func TestUpgradePreservesCompatibleUserChoices(t *testing.T) {
	current := Installed{Definition: versionedDefinition("1.0.0"), Source: "original.json", Values: map[string]string{"keep": "custom", "color": "green", "number": "2", "removed": "old"}}
	current.Definition.Shortcut = "q"
	current.Definition.UpdateURL = "https://example.com/original.json"
	current.Definition.Settings = []Setting{
		{ID: "keep", Name: "Keep", Type: "text", Default: "old"},
		{ID: "color", Name: "Color", Type: "choice", Default: "red", Options: []string{"red", "green"}},
		{ID: "number", Name: "Number", Type: "text", Default: "1"},
		{ID: "removed", Name: "Removed", Type: "text", Default: "old"},
	}
	next := versionedDefinition("1.1.0")
	next.Shortcut = "z"
	next.Settings = []Setting{
		{ID: "keep", Name: "Keep", Type: "text", Default: "new"},
		{ID: "color", Name: "Color", Type: "choice", Default: "blue", Options: []string{"blue", "red"}},
		{ID: "number", Name: "Number", Type: "number", Default: 3},
		{ID: "added", Name: "Added", Type: "bool", Default: true},
	}
	updated, err := current.Upgrade(next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.Source != current.Source || updated.Definition.Shortcut != "q" || updated.Definition.Version != "1.1.0" || updated.Definition.UpdateURL != current.Definition.UpdateURL {
		t.Fatalf("installation choices were lost: %+v", updated)
	}
	if !maps.Equal(updated.Values, map[string]string{"keep": "custom", "number": "2"}) {
		t.Fatalf("incompatible or missing settings retained: %v", updated.Values)
	}
	if updated.Value(updated.Definition.Settings[1]) != "blue" || updated.Value(updated.Definition.Settings[3]) != "true" {
		t.Fatal("new defaults not used for incompatible or new settings")
	}
	next.Settings[0].Default = "changed after installation"
	updated.Values["keep"] = "changed after installation"
	if updated.Definition.Settings[0].Default != "new" || current.Values["keep"] != "custom" || current.Definition.Version != "1.0.0" {
		t.Fatal("upgrade aliased mutable installation or preview state")
	}
}

func TestUpgradeUpdateURLAndEmptyShortcut(t *testing.T) {
	current := Installed{Enabled: true, Definition: versionedDefinition("1.0.0")}
	current.Definition.UpdateURL = "https://example.com/original.json"
	next := versionedDefinition("1.1.0")
	next.Shortcut, next.UpdateURL = "z", "https://example.com/next.json"
	updated, err := current.Upgrade(next)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.Definition.Shortcut != "" || updated.Definition.UpdateURL != next.UpdateURL {
		t.Fatalf("shortcut preference or new update URL was lost: %+v", updated)
	}
}

func TestUpgradeRejectsNonUpdates(t *testing.T) {
	current := Installed{Enabled: true, Definition: versionedDefinition("1.0.0")}
	for _, value := range []string{"1.0.0", "1.0.0+other-build", "0.9.0", "1.0.0-rc.1", "bad"} {
		if _, err := current.Upgrade(versionedDefinition(value)); err == nil {
			t.Errorf("accepted version %q over 1.0.0", value)
		}
	}
	next := versionedDefinition("1.1.0")
	next.ID = "different"
	if _, err := current.Upgrade(next); err == nil {
		t.Fatal("accepted a different addon ID")
	}
	next = versionedDefinition("1.1.0")
	next.FormatVersion = 2
	if _, err := current.Upgrade(next); err == nil {
		t.Fatal("accepted an unsupported manifest format")
	}
}
