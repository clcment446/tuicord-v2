package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestLoadFromMissingWritesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom missing: %v", err)
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v, want Default()", cfg)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("default file not written: %v", err)
	}
}

func TestLoadFromLayersOverDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[layout]\nchannels_width = 30\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.Layout.ChannelsWidth != 30 {
		t.Errorf("ChannelsWidth = %d, want 30", cfg.Layout.ChannelsWidth)
	}
	// Unspecified fields keep their defaults.
	if cfg.Layout.GuildsWidth != Default().Layout.GuildsWidth {
		t.Errorf("GuildsWidth = %d, want default %d", cfg.Layout.GuildsWidth, Default().Layout.GuildsWidth)
	}
}

func TestLoadElementLayoutPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[layout.elements.guilds]\nvisible = false\nwidth = 8\nmin_width = 3\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	policy := cfg.Layout.Element("guilds")
	if policy.Visible == nil || *policy.Visible {
		t.Fatalf("Visible = %v, want false", policy.Visible)
	}
	if policy.Width != 8 || policy.MinWidth != 3 {
		t.Fatalf("policy = %+v, want width 8/min 3", policy)
	}
}

func TestAuthPreferredModeRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := Default()
	want.Auth.PreferredMode = AuthModeBrowser
	if err := saveTo(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Auth.PreferredMode != AuthModeBrowser {
		t.Fatalf("preferred auth mode = %q, want %q", got.Auth.PreferredMode, AuthModeBrowser)
	}
}

func TestLoadFromMalformedReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("this is not = = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFrom(path); err == nil {
		t.Error("expected error for malformed toml, got nil")
	}
}

func TestParseColor(t *testing.T) {
	tests := []struct {
		in   string
		want screen.Color
		ok   bool
	}{
		{"#ffffff", screen.RGB(255, 255, 255), true},
		{"5865f2", screen.RGB(0x58, 0x65, 0xf2), true},
		{"", screen.Color{}, true},
		{"#fff", screen.Color{}, false},
		{"#gggggg", screen.Color{}, false},
	}
	for _, tt := range tests {
		got, err := ParseColor(tt.in)
		if tt.ok && err != nil {
			t.Errorf("ParseColor(%q) error: %v", tt.in, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ParseColor(%q) expected error", tt.in)
		}
		if tt.ok && got != tt.want {
			t.Errorf("ParseColor(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestDefaultColorsUseCatppuccinLattePalette(t *testing.T) {
	if !Default().Colors.Enabled {
		t.Fatal("Catppuccin Latte should be enabled by default")
	}
	styles := Default().Colors.Styles()
	bg := screen.RGB(0xef, 0xf1, 0xf5) // Catppuccin Latte base
	if styles.Background != bg {
		t.Errorf("default background = %+v, want %+v", styles.Background, bg)
	}
	if !styles.Text.Fg.Set() {
		t.Error("default text color should be set from the vivian palette")
	}
	if styles.Text.Bg != bg {
		t.Errorf("default text bg = %+v, want the palette background", styles.Text.Bg)
	}
	if styles.Accent.Attrs&screen.Bold == 0 {
		t.Error("accent style should be bold")
	}
	if styles.Selection.Attrs&screen.Reverse != 0 || !styles.Selection.Bg.Set() {
		t.Errorf("default selection should be a configured bg without reverse, got %+v", styles.Selection)
	}
	if styles.Border.Bg != bg {
		t.Errorf("default border bg = %+v, want the palette background", styles.Border.Bg)
	}
}

func TestDefaultHeadersUseLazyVimCatppuccinRainbow(t *testing.T) {
	styles := CellStyles(Default().Colors.Styles(), nil)
	want := []screen.Color{
		screen.RGB(0xd2, 0x0f, 0x39), // red
		screen.RGB(0xfe, 0x64, 0x0b), // peach
		screen.RGB(0xdf, 0x8e, 0x1d), // yellow
		screen.RGB(0x40, 0xa0, 0x2b), // green
		screen.RGB(0x20, 0x9f, 0xb5), // sapphire
		screen.RGB(0x88, 0x39, 0xef), // mauve
	}
	for level, color := range want {
		got := styles[fmt.Sprintf("messages.header%d", level+1)].Fg
		if got != color {
			t.Errorf("header%d color = %+v, want %+v", level+1, got, color)
		}
	}
}

func TestCustomPaletteHeadersUseSemanticColors(t *testing.T) {
	palette := Default().Colors.Styles()
	palette.Accent.Fg = screen.RGB(1, 2, 3)
	palette.Error.Fg = screen.RGB(4, 5, 6)
	styles := CellStyles(palette, nil)
	if got := styles["messages.header1"].Fg; got != palette.Error.Fg {
		t.Fatalf("header1 color = %+v, want custom error %+v", got, palette.Error.Fg)
	}
	if got := styles["messages.header2"].Fg; got != palette.Accent.Fg {
		t.Fatalf("header2 color = %+v, want custom accent %+v", got, palette.Accent.Fg)
	}
}

func TestCustomColorsRequireExplicitOptIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[colors]\naccent = \"#ff0000\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Colors.Accent == "#ff0000" {
		t.Fatal("custom accent applied without colors.enabled = true")
	}
	if cfg.Colors.Accent != Default().Colors.Accent {
		t.Fatalf("accent = %q, want built-in %q", cfg.Colors.Accent, Default().Colors.Accent)
	}

	if err := os.WriteFile(path, []byte(contents+"enabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Colors.Accent != "#ff0000" {
		t.Fatalf("opted-in accent = %q, want #ff0000", cfg.Colors.Accent)
	}
}

func TestLoadColorsConfUsesExactRuleOverWildcard(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	colorsPath := filepath.Join(dir, "colors.conf")
	contents := "guilds.channels.bg_color=#ffffff\n" +
		"guilds.channels.fg_color=#101010\n" +
		"guilds.separators.*.bg=#ffffff\n" +
		"guilds.separators.right.fg=#0000ff // blue separator\n" +
		"guilds.separators.right,bg-color=#ff0000\n" +
		"messages.header{n}.fg=#800080\n" +
		"messages.bold.fg=#ff00ff\n" +
		"messages.bold.attrs=bold|underline\n"
	if err := os.WriteFile(colorsPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(configPath)
	if err != nil {
		t.Fatal(err)
	}
	styles := ApplyColorOverrides(Default().Colors.Styles(), cfg.ColorOverrides)
	if styles.Text.Bg != screen.RGB(255, 255, 255) || styles.Text.Fg != screen.RGB(0x10, 0x10, 0x10) {
		t.Fatalf("channel style = %+v, want configured fg/bg", styles.Text)
	}
	if styles.Border.Fg != screen.RGB(0, 0, 255) || styles.Border.Bg != screen.RGB(255, 0, 0) {
		t.Fatalf("right separator style = %+v, want exact rule over wildcard", styles.Border)
	}
	cellStyles := CellStyles(Default().Colors.Styles(), cfg.ColorOverrides)
	if cellStyles["messages.header1"].Fg != screen.RGB(0x80, 0, 0x80) || cellStyles["messages.header6"].Fg != screen.RGB(0x80, 0, 0x80) {
		t.Fatalf("header styles = %+v / %+v, want header{n} override", cellStyles["messages.header1"], cellStyles["messages.header6"])
	}
	if cellStyles["messages.bold"].Fg != screen.RGB(255, 0, 255) || cellStyles["messages.bold"].Attrs != screen.Bold|screen.Underline {
		t.Fatalf("bold cell style = %+v, want custom color and attrs", cellStyles["messages.bold"])
	}
}

func TestDefaultAccessibilityPreservesMouseAndSkipsSplitFocus(t *testing.T) {
	cfg := Default()
	if !cfg.Accessibility.MouseOn {
		t.Fatal("mouse should be enabled by default")
	}
	if cfg.Accessibility.FocusSplits {
		t.Fatal("split selectors should be skipped by default")
	}
	if cfg.Accessibility.VimNavigation || cfg.Accessibility.MouseBreakpointTracking || cfg.Accessibility.HighlightFocusBlock {
		t.Fatalf("Vim and mouse breakpoint tracking must be opt-in: %+v", cfg.Accessibility)
	}
}

func TestLoadFromAccessibilitySection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[accessibility]\nmouse_on = false\nfocus_splits = true\nvim_navigation = true\nmouse_breakpoint_tracking = true\nhighlight_focus_block = true\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.Accessibility.MouseOn || !cfg.Accessibility.FocusSplits || !cfg.Accessibility.VimNavigation || !cfg.Accessibility.MouseBreakpointTracking || !cfg.Accessibility.HighlightFocusBlock {
		t.Fatalf("accessibility = %+v, want explicit accessibility options enabled", cfg.Accessibility)
	}
}

func TestLoadFromTTYColorsDisplayOption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[display]\ntty_colors = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if !cfg.Display.TTYColors {
		t.Fatal("display.tty_colors was not loaded")
	}
}

func TestLoadFromRoleGradientDisplayOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[display]\nrole_gradients = true\nrole_gradient_animations = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if !cfg.Display.RoleGradients || !cfg.Display.RoleGradientAnimations {
		t.Fatalf("gradient display options were not loaded: %+v", cfg.Display)
	}
}

func TestSlashCommandIntegrationIsOptIn(t *testing.T) {
	if Default().Integrations.SlashCommands.Enabled {
		t.Fatal("slash commands must be disabled by default")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[integrations.slash_commands]\nenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Integrations.SlashCommands.Enabled {
		t.Fatal("slash_commands.enabled was not loaded")
	}
}

func TestColorsStylesUseConfiguredColors(t *testing.T) {
	styles := (Colors{
		Text:      "#dcddde",
		Muted:     "#72767d",
		Accent:    "#5865f2",
		Selection: "#4f545c",
		Border:    "#202225",
		Error:     "#ed4245",
	}).Styles()
	if styles.Accent.Fg != screen.RGB(0x58, 0x65, 0xf2) || styles.Accent.Attrs&screen.Bold == 0 {
		t.Errorf("accent style = %+v, want #5865f2 bold", styles.Accent)
	}
	if styles.Selection.Bg != screen.RGB(0x4f, 0x54, 0x5c) || styles.Selection.Attrs&screen.Reverse != 0 {
		t.Errorf("selection style = %+v, want configured bg without reverse", styles.Selection)
	}
	if styles.Error.Fg != screen.RGB(0xed, 0x42, 0x45) {
		t.Errorf("error fg = %+v, want #ed4245", styles.Error.Fg)
	}
}

func TestLoadFromColorsSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[colors]\nenabled = true\naccent = \"#5865f2\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.Colors.Accent != "#5865f2" {
		t.Fatalf("accent color = %q, want #5865f2", cfg.Colors.Accent)
	}
}

func TestLoadFromLegacyThemeSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := "[theme]\nenabled = true\naccent = \"#5865f2\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if cfg.Colors.Accent != "#5865f2" {
		t.Fatalf("legacy accent color = %q, want #5865f2", cfg.Colors.Accent)
	}
}
