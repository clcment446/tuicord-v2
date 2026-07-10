// Package config loads and persists the tuicord user configuration.
//
// Configuration lives at ~/.config/tuicord/config.toml. Load returns sane
// defaults when the file is absent and writes a commented default file on first
// run, so a fresh install starts with a discoverable, editable config.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// AppName is the config/keyring namespace.
const AppName = "tuicord"

// Layout controls sidebar widths and the members panel.
type Layout struct {
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
// Default() ships a full dark palette (the "vivian" theme) so the client looks
// cohesive out of the box on any terminal. A user may override any field in
// config.toml; an explicitly empty field falls back to the terminal default for
// that role.
type Colors struct {
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
}

// Config is the full user configuration.
type Config struct {
	Layout  Layout  `toml:"layout"`
	Keys    Keys    `toml:"keys"`
	Colors  Colors  `toml:"colors"`
	Nitro   Nitro   `toml:"nitro"`
	Display Display `toml:"display"`
}

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
		Nitro: Nitro{Fake: true},
		// The "miyabi" palette — Terafox teal dark theme.
		Colors: Colors{
			Background: "#152528",
			Text:       "#e6eaea",
			Muted:      "#3f585d",
			Accent:     "#4d9c9f",
			Selection:  "#1d3337",
			Border:     "#233134",
			Error:      "#eb746b",
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

func loadFrom(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if werr := writeDefault(path); werr != nil {
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
	return cfg, nil
}

// overlayColors returns base with every non-empty field of over applied on top.
func overlayColors(base, over Colors) Colors {
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
	return toml.NewEncoder(f).Encode(Default())
}
