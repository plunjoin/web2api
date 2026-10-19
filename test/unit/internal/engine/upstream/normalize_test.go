//go:build web2api_unit

package upstream

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.example.com", "https://api.example.com"},
		{"https://api.example.com/", "https://api.example.com"},
		{"https://api.example.com/v1", "https://api.example.com"},
		{"https://api.example.com/v1/", "https://api.example.com"},
		{"https://openrouter.ai/api/v1", "https://openrouter.ai/api"},
		{"  https://x.com/v1  ", "https://x.com"},
	}
	for _, tc := range cases {
		if got := NormalizeBaseURL(tc.in); got != tc.want {
			t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
