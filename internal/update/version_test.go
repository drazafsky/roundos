package update

import (
	"strings"
	"testing"
)

func TestVersionValidation(t *testing.T) {
	for _, text := range []string{"0.5.1", "v0.6.0", "1.2.3-dev.42.gabcdef0", "1.2.3-rc.1+build.01", "1.2.3-0", "1.2.3-01a"} {
		if _, err := parseVersion(text); err != nil {
			t.Errorf("rejected valid version %q: %v", text, err)
		}
	}
	for _, text := range []string{"", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3-rc..1", "1.2.3+", "vV1.2.3", "1.2.3\n", "99999999999999999999999.2.3", strings.Repeat("1", 129)} {
		if _, err := parseVersion(text); err == nil {
			t.Errorf("accepted invalid version %q", text)
		}
	}
}
