package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesXDGConfigHomeAndCreatesLuaFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, startup, err := LoadStartup()
	if err != nil {
		t.Fatalf("LoadStartup: %v", err)
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v, want Default()", cfg)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, AppName, "config.lua")
	if path != want || startup.LuaPath != want || !startup.ExecuteLua || startup.Legacy {
		t.Fatalf("path/startup = %q/%+v, want fresh executable Lua", path, startup)
	}
	template, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	for _, want := range []string{"tuicord.configure", "tuicord.theme", "palette =", "styles =", "tuicord.use_theme"} {
		if !strings.Contains(string(template), want) {
			t.Errorf("generated config.lua missing %q", want)
		}
	}
	plugins, err := PluginsDir()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(plugins); err != nil || !info.IsDir() {
		t.Fatalf("plugins directory not created: %v", err)
	}
	for _, legacy := range []string{"config.toml", "colors.conf"} {
		if _, err := os.Stat(filepath.Join(dir, AppName, legacy)); !os.IsNotExist(err) {
			t.Fatalf("fresh install created legacy %s", legacy)
		}
	}
}

func TestLegacyTOMLMigratesAtomicallyWithoutExecutingOrRemovingInputs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	appDir := filepath.Join(dir, AppName)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyConfig := "[layout]\nchannels_width = 37\n[colors]\nenabled = true\naccent = \"#123456\"\n"
	legacyColors := "messages.author.fg=#abcdef\n"
	if err := os.WriteFile(filepath.Join(appDir, "config.toml"), []byte(legacyConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "colors.conf"), []byte(legacyColors), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, startup, err := LoadStartup()
	if err != nil {
		t.Fatal(err)
	}
	if !startup.Legacy || startup.ExecuteLua || cfg.Layout.ChannelsWidth != 37 || cfg.Colors.Accent != "#123456" {
		t.Fatalf("legacy startup/cfg = %+v / %+v", startup, cfg)
	}
	if cfg.ColorOverrides == nil || !cfg.ColorOverrides.HasOverride("messages.author") {
		t.Fatal("legacy colors.conf was not loaded")
	}
	generated, err := os.ReadFile(filepath.Join(appDir, "config.lua"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tuicord.configure", "channels_width = 37", "legacy-migrated", "messages.author", "tuicord.use_theme"} {
		if !strings.Contains(string(generated), want) {
			t.Errorf("migration missing %q", want)
		}
	}
	for name, want := range map[string]string{"config.toml": legacyConfig, "colors.conf": legacyColors} {
		got, err := os.ReadFile(filepath.Join(appDir, name))
		if err != nil || string(got) != want {
			t.Fatalf("legacy %s changed: %q, %v", name, got, err)
		}
	}
	_, next, err := LoadStartup()
	if err != nil || !next.ExecuteLua || next.Legacy {
		t.Fatalf("second startup = %+v, %v", next, err)
	}
}

func TestPrimaryAndLegacyPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	checks := []struct {
		path func() (string, error)
		base string
	}{{Path, "config.lua"}, {ConfigLuaPath, "config.lua"}, {LegacyTOMLPath, "config.toml"}, {LegacyColorsPath, "colors.conf"}, {PluginsDir, "plugins"}}
	for _, check := range checks {
		got, err := check.path()
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(dir, AppName, check.base) {
			t.Errorf("path = %q, want %s", got, check.base)
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
