//go:build web2api_unit

package version

import (
	"regexp"
	"strings"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct{ injected, embedded, want string }{
		{"v1.2.3", "v0.2.6\n", "v1.2.3"},
		{"dev", "v0.2.6\n", "v0.2.6"},
		{"", "v0.2.6", "v0.2.6"},
		{"dev", "", "dev"},
	} {
		if got := resolveVersion(tc.injected, tc.embedded); got != tc.want {
			t.Fatalf("resolveVersion(%q, %q) = %q, want %q", tc.injected, tc.embedded, got, tc.want)
		}
	}
}

// Docker builds without a VERSION build-arg must still report the release.
func TestEmbeddedReleaseVersion(t *testing.T) {
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(strings.TrimSpace(releaseVersion)) {
		t.Fatalf("internal/version/VERSION must hold a release tag, got %q", releaseVersion)
	}
}
