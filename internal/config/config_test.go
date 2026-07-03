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

func TestThemeStyles(t *testing.T) {
	styles := Default().Theme.Styles()
	if styles.Accent.Attrs&screen.Bold == 0 {
		t.Error("accent style should be bold")
	}
	if styles.Error.Fg != screen.RGB(0xed, 0x42, 0x45) {
		t.Errorf("error fg = %+v, want #ed4245", styles.Error.Fg)
	}
}
