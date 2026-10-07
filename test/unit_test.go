package test

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Existing private-package tests are physically centralized in test/unit.
// A Go overlay compiles them in their original packages without production-only
// exports or copying tests back into the source tree.
func TestInternalPackages(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	overlay := struct{ Replace map[string]string }{Replace: map[string]string{}}
	tmp := t.TempDir()
	unit := filepath.Join(root, "test", "unit")
	err = filepath.WalkDir(unit, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(unit, name)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		body = []byte(strings.TrimPrefix(strings.ReplaceAll(string(body), "\r\n", "\n"), "//go:build web2api_unit\n\n"))
		physical := filepath.Join(tmp, relative)
		if err := os.MkdirAll(filepath.Dir(physical), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(physical, body, 0o600); err != nil {
			return err
		}
		overlay.Replace[filepath.Join(root, relative)] = physical
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(overlay)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"test", "-overlay", filename, "-count=1", "./internal/..."}
	if raceEnabled {
		args = append(args, "-race")
	}
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("centralized unit suite: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
