//go:build web2api_unit

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesDefaultsWhenConfigIsMissing(t *testing.T) {
	t.Setenv("WEB2API_API_KEYS", "sk-from-compose")
	t.Setenv("WEB2API_DB_PATH", "/app/data/web2api.db")

	cfg, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.APIKeys[0] != "sk-from-compose" || cfg.Server.DBPath != "/app/data/web2api.db" {
		t.Fatalf("environment overrides were not applied: %#v", cfg.Server)
	}
	if !cfg.EngineA.Enabled {
		t.Fatalf("unexpected engine A defaults: %#v", cfg.EngineA)
	}
	if !cfg.EngineB.Enabled || cfg.EngineB.Mode != "native" || cfg.EngineB.AuthStates != "auth" {
		t.Fatalf("unexpected engine B defaults: %#v", cfg.EngineB)
	}
}

func TestLoadUsesDefaultsWhenConfigPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error for directory: %v", err)
	}
	if cfg.Server.Listen != "0.0.0.0:8800" {
		t.Fatalf("unexpected default listen address: %q", cfg.Server.Listen)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("test directory disappeared: %v", err)
	}
}
