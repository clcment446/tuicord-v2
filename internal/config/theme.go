package config

import (
	"fmt"
	"sort"
	"strings"

	"awesomeProject/internal/tui/screen"
)

// Theme is a fully resolved, validated runtime theme. Palette always contains
// all seven semantic colors; partial declarations inherit from Default rather
// than from whichever theme happened to be active previously. Styles contains
// validated semantic selector rules using the colors.conf rule model.
type Theme struct {
	Palette Colors
	Styles  ColorOverrides
}

var paletteKeys = map[string]func(*Colors) *string{
	"background": func(c *Colors) *string { return &c.Background },
	"text":       func(c *Colors) *string { return &c.Text },
	"muted":      func(c *Colors) *string { return &c.Muted },
	"accent":     func(c *Colors) *string { return &c.Accent },
	"selection":  func(c *Colors) *string { return &c.Selection },
	"border":     func(c *Colors) *string { return &c.Border },
	"error":      func(c *Colors) *string { return &c.Error },
}

// NewTheme resolves a declarative theme. Palette and style property maps are
// copied. Unknown palette keys, unsupported style properties, and malformed
// colors are errors; no invalid color is replaced with terminal-default.
func NewTheme(palette map[string]string, styles map[string]map[string]string) (Theme, error) {
	resolved := Default().Colors
	resolved.Enabled = true
	for key, value := range palette {
		field, ok := paletteKeys[key]
		if !ok {
			return Theme{}, fmt.Errorf("palette.%s: unknown color", key)
		}
		if strings.TrimSpace(value) == "" {
			return Theme{}, fmt.Errorf("palette.%s: color must not be empty", key)
		}
		if _, err := ParseColor(value); err != nil {
			return Theme{}, fmt.Errorf("palette.%s: %w", key, err)
		}
		*field(&resolved) = value
	}
	if err := ValidateColors(resolved); err != nil {
		return Theme{}, err
	}

	overrides := ColorOverrides{Rules: make(map[string]ColorRule)}
	selectors := make([]string, 0, len(styles))
	for selector := range styles {
		selectors = append(selectors, selector)
	}
	sort.Strings(selectors)
	for _, selector := range selectors {
		properties := make([]string, 0, len(styles[selector]))
		for property := range styles[selector] {
			properties = append(properties, property)
		}
		sort.Strings(properties)
		for _, property := range properties {
			if err := overrides.SetProperty(selector, property, styles[selector][property]); err != nil {
				return Theme{}, fmt.Errorf("styles.%s.%s: %w", selector, property, err)
			}
		}
	}
	return Theme{Palette: resolved, Styles: overrides}, nil
}

// ThemeFromConfig returns the current Config palette and semantic rules as a
// validated theme. It is useful for the built-in/unselected runtime theme.
func ThemeFromConfig(cfg Config) (Theme, error) {
	palette := map[string]string{
		"background": cfg.Colors.Background,
		"text":       cfg.Colors.Text,
		"muted":      cfg.Colors.Muted,
		"accent":     cfg.Colors.Accent,
		"selection":  cfg.Colors.Selection,
		"border":     cfg.Colors.Border,
		"error":      cfg.Colors.Error,
	}
	theme, err := NewTheme(palette, nil)
	if err != nil {
		return Theme{}, err
	}
	if cfg.ColorOverrides != nil {
		theme.Styles = cfg.ColorOverrides.Clone()
	}
	return theme, nil
}

// ApplyTheme projects a resolved theme into Config before UI construction.
func ApplyTheme(cfg *Config, theme Theme) {
	if cfg == nil {
		return
	}
	cfg.Colors = theme.Palette
	overrides := theme.Styles.Clone()
	cfg.ColorOverrides = &overrides
}

// ValidateColors checks every configured palette field and returns its semantic
// path on failure.
func ValidateColors(colors Colors) error {
	values := map[string]string{
		"background": colors.Background,
		"text":       colors.Text,
		"muted":      colors.Muted,
		"accent":     colors.Accent,
		"selection":  colors.Selection,
		"border":     colors.Border,
		"error":      colors.Error,
	}
	for _, key := range []string{"background", "text", "muted", "accent", "selection", "border", "error"} {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("%s: color must not be empty", key)
		}
		if _, err := ParseColor(values[key]); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

// Clone returns a deep copy suitable for installing into a live shared style
// target without aliasing a registry entry.
func (o ColorOverrides) Clone() ColorOverrides {
	out := ColorOverrides{Rules: make(map[string]ColorRule, len(o.Rules))}
	for selector, rule := range o.Rules {
		out.Rules[selector] = rule
	}
	return out
}

// Replace repopulates this override set in place. Existing Styles values keep
// the same pointer and see the new rules on the next draw.
func (o *ColorOverrides) Replace(next ColorOverrides) {
	if o == nil {
		return
	}
	if o.Rules == nil {
		o.Rules = make(map[string]ColorRule)
	}
	for selector := range o.Rules {
		delete(o.Rules, selector)
	}
	for selector, rule := range next.Rules {
		o.Rules[selector] = rule
	}
}

// colorHex renders a parsed color for migration output.
func colorHex(c screen.Color) string {
	if !c.Set() {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
