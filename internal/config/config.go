// Package config loads and persists the tuicord user configuration.
//
// Configuration lives at ~/.config/tuicord-v2/config.toml. Load returns sane
// defaults when the file is absent and writes a commented default file on first
// run, so a fresh install starts with a discoverable, editable config.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"awesomeProject/internal/tui/layout"
	"github.com/BurntSushi/toml"
)

// AppName is the config/keyring namespace.
const AppName = "tuicord-v2"

// Layout controls sidebar widths and the members panel.
type Layout struct {
	// Elements contains per-surface presentation policies. Keys are semantic UI
	// names such as "guilds", "channels", "members", "messages", and
	// "composer".
	Elements *ElementPolicies `toml:"elements"`
	// GuildsWidth is the guild rail width in columns.
	GuildsWidth int `toml:"guilds_width"`
	// ChannelsWidth is the channel sidebar width in columns.
	ChannelsWidth int `toml:"channels_width"`
	// MembersWidth is the members sidebar width in columns.
	MembersWidth int `toml:"members_width"`
	// MembersAutoHide hides the members panel below MembersHideBelow columns.
	MembersAutoHide bool `toml:"members_auto_hide"`
	// MembersHideBelow is the terminal width under which members auto-hides.
	MembersHideBelow int `toml:"members_hide_below"`
}

// ElementLayout controls whether a named UI surface is rendered and its size
// along the containing layout's main axis. Zero dimensions inherit the widget's
// built-in policy.
type ElementLayout struct {
	Visible   *bool `toml:"visible"`
	Width     int   `toml:"width"`
	Height    int   `toml:"height"`
	MinWidth  int   `toml:"min_width"`
	MaxWidth  int   `toml:"max_width"`
	MinHeight int   `toml:"min_height"`
	MaxHeight int   `toml:"max_height"`
}

// ElementPolicies is the named-surface layout table. It is held by pointer so
// Config remains comparable for callers that use it as a default-value check.
type ElementPolicies map[string]ElementLayout

// Apply projects a named element policy onto a low-level layout node.
func (e ElementLayout) Apply(node *layout.Node, dir layout.Direction) {
	if node == nil {
		return
	}
	if e.Visible != nil {
		node.Hidden = !*e.Visible
	}
	if dir == layout.Row {
		if e.Width > 0 {
			node.Basis, node.Grow = e.Width, 0
		}
		if e.MinWidth > 0 {
			node.Min = e.MinWidth
		}
		if e.MaxWidth > 0 {
			node.Max = e.MaxWidth
		}
		return
	}
	if e.Height > 0 {
		node.Basis, node.Grow = e.Height, 0
	}
	if e.MinHeight > 0 {
		node.Min = e.MinHeight
	}
	if e.MaxHeight > 0 {
		node.Max = e.MaxHeight
	}
}

// Element returns a configured policy, or the zero policy when absent.
func (l Layout) Element(name string) ElementLayout {
	if l.Elements == nil {
		return ElementLayout{}
	}
	return (*l.Elements)[name]
}

// Keys maps actions to key names understood by the UI.
type Keys struct {
	QuickSwitcher string `toml:"quick_switcher"`
	Help          string `toml:"help"`
	NextPanel     string `toml:"next_panel"`
	FocusComposer string `toml:"focus_composer"`
	// Picker opens the emoji/sticker picker over the composer.
	Picker string `toml:"picker"`
}

// Nitro controls how the picker sends emoji and stickers the account cannot use
// natively.
type Nitro struct {
	// Fake enables the "fake nitro" fallback: instead of a native emoji/sticker
	// send, the picker inserts the CDN URL, which tuicord renders back inline.
	Fake bool `toml:"fake"`
}

// Colors holds hex colors (e.g. "#5865F2") applied to the widget tree.
//
// Default() ships Catppuccin Latte so the client looks cohesive out of the box
// on any terminal. User color overrides are opt-in through Enabled.
type Colors struct {
	// Enabled opts into replacing the built-in palette with these values.
	Enabled bool `toml:"enabled"`
	// Background is the base fill painted behind every panel.
	Background string `toml:"background"`
	Text       string `toml:"text"`
	Muted      string `toml:"muted"`
	Accent     string `toml:"accent"`
	Selection  string `toml:"selection"`
	Border     string `toml:"border"`
	Error      string `toml:"error"`
}

// Display controls presentation details that are not colors.
type Display struct {
	// ASCII forces ASCII-only glyphs for channel badges and sidebar markers,
	// for terminals or fonts without good Unicode symbol coverage. The client
	// also switches to ASCII automatically when NO_COLOR is set in the
	// environment.
	ASCII bool `toml:"ascii"`
	// TTYColors restricts UI colors to the terminal's standard 16-color palette.
	TTYColors bool `toml:"tty_colors"`
}

// Auth controls how interactive Discord authentication is presented.
type Auth struct {
	// PreferredMode is "tui" for an in-terminal browser surface or "browser"
	// for a full Firefox window. Empty means the login prompt chooses the
	// default terminal mode first.
	PreferredMode string `toml:"preferred_mode"`
}

// Accessibility controls alternate input paths for users who prefer or need
// keyboard-first navigation.
type Accessibility struct {
	MouseOn                 bool `toml:"mouse_on"`
	FocusSplits             bool `toml:"focus_splits"`
	VimNavigation           bool `toml:"vim_navigation"`
	MouseBreakpointTracking bool `toml:"mouse_breakpoint_tracking"`
	HighlightFocusBlock     bool `toml:"highlight_focus_block"`
}

// SlashCommands controls experimental user-client application-command support.
// It is disabled by default because Discord does not document this client-side
// protocol as a supported public integration surface.
type SlashCommands struct {
	Enabled bool `toml:"enabled"`
}

// Integrations groups optional external-service features.
type Integrations struct {
	SlashCommands SlashCommands `toml:"slash_commands"`
}

// Config is the full user configuration.
type Config struct {
	Layout         Layout          `toml:"layout"`
	Keys           Keys            `toml:"keys"`
	Colors         Colors          `toml:"colors"`
	Nitro          Nitro           `toml:"nitro"`
	Display        Display         `toml:"display"`
	Auth           Auth            `toml:"auth"`
	Accessibility  Accessibility   `toml:"accessibility"`
	Integrations   Integrations    `toml:"integrations"`
	ColorOverrides *ColorOverrides `toml:"-"`
}

// ColorOverrides contains the selector-based overrides loaded from
// colors.conf. It is intentionally separate from the built-in palette.
type ColorOverrides struct {
	Rules map[string]ColorRule
}

const (
	AuthModeTUI     = "tui"
	AuthModeBrowser = "browser"
)

// Default returns the built-in configuration used when no file exists and as
// the base that a user's file is decoded over.
func Default() Config {
	return Config{
		Layout: Layout{
			GuildsWidth:      4,
			ChannelsWidth:    24,
			MembersWidth:     20,
			MembersAutoHide:  true,
			MembersHideBelow: 120,
		},
		Keys: Keys{
			QuickSwitcher: "ctrl+k",
			Help:          "ctrl+/",
			NextPanel:     "tab",
			FocusComposer: "esc",
			Picker:        "ctrl+e",
		},
		Nitro:         Nitro{Fake: true},
		Accessibility: Accessibility{MouseOn: true},
		// Catppuccin Latte: the light variant of the Catppuccin palette.
		Colors: Colors{
			Background: "#eff1f5",
			Text:       "#4c4f69",
			Muted:      "#8c8fa1",
			Accent:     "#1e66f5",
			Selection:  "#ccd0da",
			Border:     "#bcc0cc",
			Error:      "#d20f39",
		},
	}
}

// Path returns the config file path, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppName, "config.toml"), nil
}

// Load reads the config file, layering it over Default. When the file does not
// exist it writes the default file and returns Default. Decode errors are
// returned so the user can fix a malformed file rather than silently losing it.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return loadFrom(path)
}

// Save writes the current configuration to the user config path.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return saveTo(path, cfg)
}

func saveTo(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.toml-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func loadFrom(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if werr := writeDefault(path); werr != nil {
			return cfg, werr
		}
		if werr := writeColorsTemplate(filepath.Join(filepath.Dir(path), "colors.conf")); werr != nil {
			return cfg, werr
		}
		if werr := loadOptionalColorOverrides(&cfg, path); werr != nil {
			return cfg, werr
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	// Migrate the legacy [theme] section: when the user has not set [colors]
	// (so cfg.Colors is still the defaults), overlay any colors from [theme].
	var legacy struct {
		Theme Colors `toml:"theme"`
	}
	if err := toml.Unmarshal(data, &legacy); err == nil && cfg.Colors == Default().Colors && legacy.Theme != (Colors{}) {
		cfg.Colors = overlayColors(cfg.Colors, legacy.Theme)
	}
	// Custom colors must be explicitly enabled so an old colors section cannot
	// silently replace the built-in palette after a theme upgrade.
	var sections struct {
		Colors struct {
			Enabled *bool `toml:"enabled"`
		} `toml:"colors"`
		Theme struct {
			Enabled *bool `toml:"enabled"`
		} `toml:"theme"`
	}
	if err := toml.Unmarshal(data, &sections); err == nil && cfg.Colors != Default().Colors {
		colorsEnabled := sections.Colors.Enabled != nil && *sections.Colors.Enabled
		themeEnabled := sections.Theme.Enabled != nil && *sections.Theme.Enabled
		if !colorsEnabled && !themeEnabled {
			cfg.Colors = Default().Colors
		}
	}
	if err := loadOptionalColorOverrides(&cfg, path); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadOptionalColorOverrides(cfg *Config, configPath string) error {
	path := filepath.Join(filepath.Dir(configPath), "colors.conf")
	overrides, err := loadColorOverrides(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if len(overrides.Rules) == 0 {
		return nil
	}
	cfg.ColorOverrides = &overrides
	return nil
}

// overlayColors returns base with every non-empty field of over applied on top.
func overlayColors(base, over Colors) Colors {
	base.Enabled = base.Enabled || over.Enabled
	set := func(dst *string, src string) {
		if src != "" {
			*dst = src
		}
	}
	set(&base.Background, over.Background)
	set(&base.Text, over.Text)
	set(&base.Muted, over.Muted)
	set(&base.Accent, over.Accent)
	set(&base.Selection, over.Selection)
	set(&base.Border, over.Border)
	set(&base.Error, over.Error)
	return base
}

func writeDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		// Lost a race or file appeared; not fatal for loading.
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	_, err = f.WriteString(defaultConfigTemplate)
	return err
}

func writeColorsTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	defer f.Close()
	_, err = f.WriteString(colorsTemplate())
	return err
}

const defaultConfigTemplate = `# tuicord-v2 configuration
#
# This file is generated on first launch. Values below are the built-in
# defaults; uncomment or edit them to customize the client.

[layout]
guilds_width = 4
channels_width = 24
members_width = 20
members_auto_hide = true
members_hide_below = 120

# Named surfaces can control rendering and dimensions.
# [layout.elements.guilds]
# visible = true
# width = 6
# min_width = 3
# max_width = 24
#
# [layout.elements.channels]
# visible = true
# width = 30
#
# [layout.elements.members]
# visible = false
#
# [layout.elements.composer]
# height = 5

[keys]
quick_switcher = "ctrl+k"
help = "ctrl+/"
next_panel = "tab"
focus_composer = "esc"

[colors]
# Custom palette values are opt-in. Set enabled = true to use them.
enabled = false
background = "#eff1f5"
text = "#4c4f69"
muted = "#8c8fa1"
accent = "#1e66f5"
selection = "#ccd0da"
border = "#bcc0cc"
error = "#d20f39"

[display]
# Restrict emitted colors to the terminal's standard 16-color palette.
tty_colors = false
ascii = false

[auth]
# preferred_mode = "tui"   # "tui" or "browser"

[accessibility]
mouse_on = true
focus_splits = false
vim_navigation = false
mouse_breakpoint_tracking = false
highlight_focus_block = false

[nitro]
fake = true

[integrations.slash_commands]
enabled = false

# Fine-grained cell colors live beside this file in colors.conf.
# Example:
# messages.author.fg=#ff00ff
# messages.link.prettyLink.fg=#0000ff
# messages.header{n}.attrs=bold|underline
# messages.*.bg=#ffffff
`
