package addon

import (
	"regexp"
	"strings"
)

var semanticVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type version struct {
	core       [3]string
	prerelease []string
}

func parseVersion(value string) (version, bool) {
	match := semanticVersion.FindStringSubmatch(value)
	if match == nil {
		return version{}, false
	}
	v := version{core: [3]string{match[1], match[2], match[3]}}
	if match[4] != "" {
		v.prerelease = strings.Split(match[4], ".")
		for _, part := range v.prerelease {
			if numeric(part) && len(part) > 1 && part[0] == '0' {
				return version{}, false
			}
		}
	}
	return v, true
}

func numeric(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func compareNumber(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

// Newer compares semantic version precedence. Build metadata does not affect
// precedence, and an invalid version never qualifies as an update.
func Newer(latest, current string) bool {
	a, ok := parseVersion(latest)
	if !ok {
		return false
	}
	b, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := range a.core {
		if cmp := compareNumber(a.core[i], b.core[i]); cmp != 0 {
			return cmp > 0
		}
	}
	if len(a.prerelease) == 0 || len(b.prerelease) == 0 {
		return len(a.prerelease) == 0 && len(b.prerelease) != 0
	}
	for i := 0; i < min(len(a.prerelease), len(b.prerelease)); i++ {
		ap, bp := a.prerelease[i], b.prerelease[i]
		if ap == bp {
			continue
		}
		an, bn := numeric(ap), numeric(bp)
		if an && bn {
			return compareNumber(ap, bp) > 0
		}
		if an != bn {
			return !an
		}
		return ap > bp
	}
	return len(a.prerelease) > len(b.prerelease)
}
