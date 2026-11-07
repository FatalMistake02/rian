package addon

import (
	"encoding/json"
	"strings"
	"testing"
)

const currentManifest = `{"format_version":1,"version":"1.2.3","id":"test","name":"Test","pages":[{"id":"notes","name":"Notes"}]}`

func TestParseVersionedManifests(t *testing.T) {
	for _, tc := range []struct {
		name, data, version string
	}{
		{"current", currentManifest, "1.2.3"},
		{"prerelease", strings.Replace(currentManifest, "1.2.3", "2.0.0-rc.1+build.001", 1), "2.0.0-rc.1+build.001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.data))
			if err != nil {
				t.Fatal(err)
			}
			if d.FormatVersion != 1 || d.Version != tc.version {
				t.Fatalf("incorrect version decoding: %+v", d)
			}
			encoded, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["format_version"]) != "1" || string(fields["version"]) != `"`+tc.version+`"` {
				t.Fatalf("release version changed on roundtrip: %s", encoded)
			}
		})
	}
}

func TestParseRejectsInvalidManifests(t *testing.T) {
	for _, data := range []string{
		strings.Replace(currentManifest, `"format_version":1`, `"format_version":2`, 1),
		strings.Replace(currentManifest, `"format_version":1,`, "", 1),
		strings.Replace(currentManifest, `"format_version":1`, `"format_version":null`, 1),
		strings.Replace(currentManifest, `"version":"1.2.3"`, `"version":1`, 1),
		strings.Replace(currentManifest, `"version":"1.2.3"`, `"version":null`, 1),
		strings.Replace(currentManifest, `"version":"1.2.3",`, "", 1),
		strings.Replace(currentManifest, `"version":"1.2.3"`, `"version":"v1.2.3"`, 1),
		strings.Replace(currentManifest, `"id":"test"`, `"extra":true,"id":"test"`, 1),
		strings.Replace(currentManifest, `"id":"notes"`, `"extra":true,"id":"notes"`, 1),
		`{"version":1,"id":"test","name":"Test","pages":[{"id":"notes","name":"Notes"}]}`,
		`{"version":2,"id":"test","name":"Test","pages":[{"id":"notes","name":"Notes"}]}`,
		currentManifest + " {}",
		currentManifest + " null",
		"[" + currentManifest + "]",
		"null",
	} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("accepted invalid manifest: %s", data)
		}
	}
	if _, err := Parse([]byte(strings.Repeat(" ", maxManifestSize+1))); err == nil {
		t.Fatal("oversized manifest accepted")
	}
	if _, err := Parse([]byte(currentManifest + strings.Repeat(" ", maxManifestSize-len(currentManifest)))); err != nil {
		t.Fatalf("manifest at size limit rejected: %v", err)
	}
}

func TestDefinitionUpdateURL(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"", true}, {"https://example.com/git.json", true}, {"http://localhost:8080/addon.json", true},
		{"file:///tmp/addon.json", false}, {"https://", false}, {"example.com/addon.json", false},
		{"javascript:alert(1)", false}, {"https://example.com/\naddon.json", false},
	} {
		d := versionedDefinition("1.0.0")
		d.UpdateURL = tc.url
		if err := d.Validate(); (err == nil) != tc.valid {
			t.Errorf("update URL %q: %v", tc.url, err)
		}
	}
}
