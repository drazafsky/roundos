package update

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type version struct {
	core [3]uint64
	pre  string
}

func parseVersion(text string) (version, error) {
	var v version
	parts := versionPattern.FindStringSubmatch(text)
	if len(text) > 128 || parts == nil {
		return v, errors.New("invalid release version")
	}
	for i := range v.core {
		n, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return v, errors.New("release version number is too large")
		}
		v.core[i] = n
	}
	v.pre = parts[4]
	for _, identifier := range strings.Split(v.pre, ".") {
		if len(identifier) > 1 && identifier[0] == '0' && strings.Trim(identifier, "0123456789") == "" {
			return v, errors.New("invalid numeric prerelease identifier")
		}
	}
	return v, nil
}

// newerStable compares an already validated stable candidate to an installed
// build. Build metadata does not affect precedence; stable beats prerelease
// when the numeric core is equal.
func newerStable(candidate, installed version) bool {
	for i := range candidate.core {
		if candidate.core[i] != installed.core[i] {
			return candidate.core[i] > installed.core[i]
		}
	}
	return installed.pre != ""
}
