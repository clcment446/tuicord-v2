package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v, want Default()", cfg)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, AppName, "config.toml")
	if path != want {
		t.Errorf("Path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created by Load: %v", err)
	}
}

func TestWriteDefaultExistingFileIsNoError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[layout]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeDefault(path); err != nil {
		t.Errorf("writeDefault over existing file: %v", err)
	}
}
