package engine

import "testing"

func TestCapsForAntigravitySplitsByModelFamily(t *testing.T) {
	gemini := CapsFor("antigravity", "gemini-3.8-flash")
	if !gemini.Tools || !gemini.RetryNoToolsOn400 || gemini.FlattenToolHistory {
		t.Fatalf("gemini caps=%#v", gemini)
	}
	claude := CapsFor("antigravity", "claude-sonnet-4")
	if claude.Tools || !claude.FlattenToolHistory || claude.RetryNoToolsOn400 {
		t.Fatalf("claude caps=%#v", claude)
	}
	if !CapsFor("antigravity", "models/gemini-2.5-pro").Tools {
		t.Fatal("models/ prefix should be tolerated")
	}
}

func TestCapsForOpenAICompatFamily(t *testing.T) {
	for _, eng := range []string{"custom", "b", "codex"} {
		caps := CapsFor(eng, "whatever-model")
		if !caps.Tools || caps.FlattenToolHistory {
			t.Fatalf("%s caps=%#v", eng, caps)
		}
		if eng == "custom" && !caps.RetryNoToolsOn400 {
			t.Fatalf("custom must retry without tools on 400: %#v", caps)
		}
		if (eng == "b" || eng == "codex") && caps.RetryNoToolsOn400 {
			t.Fatalf("%s should not silently drop tools: %#v", eng, caps)
		}
	}
}

func TestCapsForUnknownEngineIsConservative(t *testing.T) {
	for _, eng := range []string{"a", "kiro", "workbuddy", "no-such-engine"} {
		caps := CapsFor(eng, "gemini-flash")
		if caps.Tools || caps.RetryNoToolsOn400 || !caps.FlattenToolHistory {
			t.Fatalf("%s caps=%#v", eng, caps)
		}
	}
}
