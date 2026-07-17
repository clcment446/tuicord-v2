package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginsEnabledDefaults(t *testing.T) {
	// A config with no [plugins] section defaults to enabled.
	if !Default().PluginsEnabled() {
		t.Fatal("plugins should be enabled by default")
	}
	cfg := Config{Plugins: &Plugins{Enabled: false, Disabled: []string{"foo"}, Grants: map[string][]string{"bar": {"fs"}}}}
	if cfg.PluginsEnabled() {
		t.Fatal("explicit enabled=false should disable plugins")
	}
	if !cfg.PluginDisabled("foo") || cfg.PluginDisabled("baz") {
		t.Fatal("PluginDisabled did not reflect the disabled list")
	}
	if got := cfg.PluginGrants("bar"); len(got) != 1 || got[0] != "fs" {
		t.Fatalf("PluginGrants(bar) = %v", got)
	}
	if got := cfg.PluginGrants("missing"); got != nil {
		t.Fatalf("PluginGrants(missing) = %v, want nil", got)
	}
}

func TestLoadCreatesPluginsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	pluginsDir, err := PluginsDir()
	if err != nil {
		t.Fatalf("PluginsDir: %v", err)
	}
	if info, err := os.Stat(pluginsDir); err != nil || !info.IsDir() {
		t.Fatalf("plugins dir not created: err=%v", err)
	}
	if want := filepath.Join(dir, AppName, "plugins"); pluginsDir != want {
		t.Fatalf("PluginsDir = %q, want %q", pluginsDir, want)
	}
}
