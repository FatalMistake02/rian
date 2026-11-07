package addon

import (
	"encoding/json"
	"fmt"
)

// Upgrade installs a newer definition while keeping the user's installation
// choices. Settings removed or made incompatible by the release use defaults.
func (i Installed) Upgrade(next Definition) (Installed, error) {
	if err := next.Validate(); err != nil {
		return Installed{}, err
	}
	if next.ID != i.Definition.ID {
		return Installed{}, fmt.Errorf("add-on ID changed from %s to %s", i.Definition.ID, next.ID)
	}
	if !Newer(next.Version, i.Definition.Version) {
		return Installed{}, fmt.Errorf("add-on version %s must be newer than installed version %s", next.Version, i.Definition.Version)
	}
	// Detach the replacement from any pending update or import preview.
	data, err := json.Marshal(next)
	if err != nil {
		return Installed{}, err
	}
	var detached Definition
	if err := json.Unmarshal(data, &detached); err != nil {
		return Installed{}, err
	}
	next = detached
	next.Shortcut = i.Definition.Shortcut
	if next.UpdateURL == "" {
		next.UpdateURL = i.Definition.UpdateURL
	}
	updated := Installed{Definition: next, Enabled: i.Enabled, Source: i.Source, Values: map[string]string{}}
	for _, setting := range next.Settings {
		if value, set := i.Values[setting.ID]; set && setting.ValidateValue(value) == nil {
			updated.Values[setting.ID] = value
		}
	}
	if err := updated.Validate(); err != nil {
		return Installed{}, err
	}
	return updated, nil
}
