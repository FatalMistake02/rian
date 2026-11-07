package updates

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"0.2.2", "0.2.1", true}, {"0.10.0", "0.9.9", true}, {"1.0.0", "0.9.0", true},
		{"0.2.1", "0.2.1", false}, {"0.2.0", "0.2.1", false}, {"bad", "0.2.1", false},
		{"0.3.0-beta", "0.2.1", false}, {"0.3.0", "dev", false},
		{"+1.0.0", "0.2.1", false}, {"01.0.0", "0.2.1", false},
		{"1.0.+0", "0.2.1", false}, {"1.0.0", "+0.2.1", false},
		{"v0.3.0", "v0.2.1", true},
	} {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q,%q) = %v", c.latest, c.current, got)
		}
	}
}

func TestCommandRejectsInvalidVersions(t *testing.T) {
	for _, version := range []string{"", "1.0", "1.0.0-beta", "+1.0.0", "01.0.0", "1.0.+0", "1.0.0\n"} {
		if _, err := Command(version); err == nil || err.Error() != "invalid release version" {
			t.Errorf("Command(%q) = %v, want invalid release version", version, err)
		}
	}
}
