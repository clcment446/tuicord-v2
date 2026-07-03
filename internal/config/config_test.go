package config

import (
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

func TestDefaultColorsUseVivianPalette(t *testing.T) {
	styles := Default().Colors.Styles()
	bg := screen.RGB(0x15, 0x25, 0x28) // miyabi (Terafox teal) background
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
	contents := "[colors]\naccent = \"#5865f2\"\n"
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
	contents := "[theme]\naccent = \"#5865f2\"\n"
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
