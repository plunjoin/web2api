package gateway

import "testing"

func TestCodexCatalogSlug(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"models/gemini-2.5-flash", "gemini-2.5-flash", true},
		{"gemini-3.5-flash", "gemini-3.5-flash", true},
		{" models/gemini-pro-latest ", "gemini-pro-latest", true},
		{"openai/gpt-4o", "", false},
		{"models/", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := codexCatalogSlug(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("codexCatalogSlug(%q) = %q,%v; want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestResponsesToolChoiceStaysJSON(t *testing.T) {
	if got := string(toChatToolChoice([]byte(`"auto"`))); got != `"auto"` {
		t.Fatalf(`tool_choice "auto" = %s`, got)
	}
	got := string(toChatToolChoice([]byte(`{"type":"function","name":"shell"}`)))
	if got != `{"function":{"name":"shell"},"type":"function"}` {
		t.Fatalf("function tool_choice = %s", got)
	}
}
