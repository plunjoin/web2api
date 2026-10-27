package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// v0.3.9 引擎改名（a→geminiweb、b→aistudio）：旧 yaml 键与旧 default_engine
// 值必须继续可读，新键优先。
func TestConfigLegacyKeysStillLoad(t *testing.T) {
	raw := `
routing:
  default_engine: a
  engine_a_models: ["my-a-model"]
  engine_b_models: ["my-b-model"]
engine_a:
  enabled: true
  refresh_seconds: 600
engine_b:
  enabled: true
  mode: native
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.DefaultEngine != "geminiweb" {
		t.Fatalf("default_engine a should normalize to geminiweb, got %q", cfg.Routing.DefaultEngine)
	}
	if len(cfg.Routing.GeminiWebModels) != 1 || cfg.Routing.GeminiWebModels[0] != "my-a-model" {
		t.Fatalf("engine_a_models should map to GeminiWebModels: %#v", cfg.Routing.GeminiWebModels)
	}
	if len(cfg.Routing.AIStudioModels) != 1 || cfg.Routing.AIStudioModels[0] != "my-b-model" {
		t.Fatalf("engine_b_models should map to AIStudioModels: %#v", cfg.Routing.AIStudioModels)
	}
	if !cfg.GeminiWeb.Enabled || cfg.GeminiWeb.RefreshSeconds != 600 {
		t.Fatalf("engine_a should map to GeminiWeb: %#v", cfg.GeminiWeb)
	}
	if !cfg.AIStudio.Enabled || cfg.AIStudio.Mode != "native" {
		t.Fatalf("engine_b should map to AIStudio: %#v", cfg.AIStudio)
	}
}

func TestConfigNewKeysWinOverLegacy(t *testing.T) {
	raw := `
engine_a:
  enabled: false
engine_geminiweb:
  enabled: true
default_engine: b
`
	// 新键与旧键同时出现时新键优先；default_engine 的 b 归一为 aistudio。
	raw = "routing:\n  default_engine: b\nengine_a:\n  enabled: false\nengine_geminiweb:\n  enabled: true\n"
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Routing.DefaultEngine != "aistudio" {
		t.Fatalf("default_engine b should normalize to aistudio, got %q", cfg.Routing.DefaultEngine)
	}
	if !cfg.GeminiWeb.Enabled {
		t.Fatal("engine_geminiweb should win over legacy engine_a")
	}
}

func TestNormalizeEngineName(t *testing.T) {
	cases := map[string]string{
		"a": "geminiweb", "gemini": "geminiweb", "GEMINIWEB": "geminiweb",
		"b": "aistudio", "AISTUDIO": "aistudio",
		"codex": "codex", "auto": "auto", "": "",
	}
	for in, want := range cases {
		if got := NormalizeEngineName(in); got != want {
			t.Fatalf("NormalizeEngineName(%q)=%q want %q", in, got, want)
		}
	}
}
