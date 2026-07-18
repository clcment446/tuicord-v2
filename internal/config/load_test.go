package config

import (
	"os"
	"path/filepath"
	"strings"
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
	template, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	for _, want := range []string{"# tuicord-v2 configuration", "[layout]", "[colors]", "tty_colors = false", "messages.author.fg"} {
		if !strings.Contains(string(template), want) {
			t.Errorf("generated config missing %q", want)
		}
	}
	colorsPath := filepath.Join(dir, AppName, "colors.conf")
	colors, err := os.ReadFile(colorsPath)
	if err != nil {
		t.Fatalf("read generated colors.conf: %v", err)
	}
	if !strings.Contains(string(colors), "# messages.author.fg=#ffffff") || !strings.Contains(string(colors), "# messages.header{n}.attrs=bold") {
		t.Error("generated colors.conf is missing semantic selector examples")
	}
	for _, line := range strings.Split(string(colors), "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("colors.conf contains active rule %q", line)
		}
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
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "[layout]\n" {
		t.Fatalf("existing config was replaced: %q", contents)
	}
}

func TestWriteColorsTemplatePreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colors.conf")
	if err := os.WriteFile(path, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeColorsTemplate(path); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "custom" {
		t.Fatalf("existing colors template was replaced: %q", contents)
	}
}
