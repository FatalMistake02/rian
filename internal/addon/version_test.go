package addon

import "testing"

func TestNewerSemanticVersionPrecedence(t *testing.T) {
	// https://semver.org/#spec-item-11
	ordered := []string{"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for i, current := range ordered {
		if Newer(current, current) {
			t.Errorf("equal version %q considered newer", current)
		}
		for _, latest := range ordered[i+1:] {
			if !Newer(latest, current) || Newer(current, latest) {
				t.Errorf("incorrect precedence: %s < %s", current, latest)
			}
		}
	}
	for _, pair := range [][2]string{{"1.0.0+build.002", "1.0.0+build.001"}, {"1.0.0-alpha+1", "1.0.0-alpha+2"}} {
		if Newer(pair[0], pair[1]) || Newer(pair[1], pair[0]) {
			t.Errorf("build metadata affected precedence: %v", pair)
		}
	}
	if !Newer("100000000000000000000.0.0", "99999999999999999999.0.0") || !Newer("1.0.0-100000000000000000000", "1.0.0-99999999999999999999") {
		t.Fatal("large version component overflowed")
	}
}

func TestSemanticVersionValidation(t *testing.T) {
	for _, value := range []string{"0.0.0", "1.2.3", "1.0.0-alpha-01", "1.0.0-0", "1.0.0+001", "1.0.0-rc.1+build.9"} {
		if _, ok := parseVersion(value); !ok {
			t.Errorf("valid version rejected: %q", value)
		}
	}
	for _, value := range []string{"", "1", "1.0", "v1.0.0", "01.0.0", "+1.0.0", "1.0.0-01", "1.0.0-alpha..1", "1.0.0-", "1.0.0+", "1.0.0+foo..bar", "1.0.0\n", "1.0.0-β"} {
		if _, ok := parseVersion(value); ok {
			t.Errorf("invalid version accepted: %q", value)
		}
		if Newer(value, "0.0.0") || Newer("99.0.0", value) {
			t.Errorf("invalid version participated in comparison: %q", value)
		}
	}
}
