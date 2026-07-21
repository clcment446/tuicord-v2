package config

import (
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestThemePartialPaletteInheritsBuiltInDefault(t *testing.T) {
	theme, err := NewTheme(
		map[string]string{"accent": "#112233"},
		map[string]map[string]string{
			"messages.author": {"fg": "#abcdef", "bold": "true"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if theme.Palette.Accent != "#112233" || theme.Palette.Background != Default().Colors.Background {
		t.Fatalf("resolved palette = %+v", theme.Palette)
	}
	rule := theme.Styles.Resolve("messages.author")
	if !rule.HasFg || rule.Fg != screen.RGB(0xab, 0xcd, 0xef) || rule.Attrs&screen.Bold == 0 {
		t.Fatalf("resolved semantic style = %+v", rule)
	}
}

func TestThemeRejectsInvalidColorsAndProperties(t *testing.T) {
	tests := []struct {
		name    string
		palette map[string]string
		styles  map[string]map[string]string
	}{
		{"unknown palette", map[string]string{"warning": "#ffffff"}, nil},
		{"invalid palette color", map[string]string{"accent": "not-a-color"}, nil},
		{"invalid style color", nil, map[string]map[string]string{"messages.author": {"fg": "#xyzxyz"}}},
		{"unknown property", nil, map[string]map[string]string{"messages.author": {"glow": "true"}}},
		{"empty selector", nil, map[string]map[string]string{"": {"bold": "true"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewTheme(tt.palette, tt.styles); err == nil {
				t.Fatal("expected invalid theme error")
			}
		})
	}
}

func TestSelectionPaletteFeedsSelectedRowSelectors(t *testing.T) {
	styles := CellStyles(Default().Colors.Styles(), nil)
	selection := styles["selection"]
	if !selection.Bg.Set() || selection.Bg != Default().Colors.Styles().Selection.Bg {
		t.Fatalf("canonical selection = %+v", selection)
	}
	for _, selector := range []string{"guilds.selected", "picker.selected", "menu.selected", "settings.selected", "forum.selected", "quick_switcher.selected"} {
		if got := styles[selector]; got.Bg != selection.Bg {
			t.Errorf("%s bg = %+v, want canonical %+v", selector, got.Bg, selection.Bg)
		}
	}
}
