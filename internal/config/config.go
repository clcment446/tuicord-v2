// Package config loads and persists the tuicord user configuration.
//
// Configuration lives at ~/.config/tuicord-v2/config.toml. Load returns sane
// defaults when the file is absent and writes a commented default file on first
// run, so a fresh install starts with a discoverable, editable config.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"awesomeProject/internal/atomicfile"
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
	// PasteImage attaches an image from the system clipboard. Defaults to
	// ctrl+v; terminals' text paste is ctrl+shift+v, so this does not shadow it.
	// Also available as the ;paste command.
	PasteImage string `toml:"paste_image"`
	// Video player controls, active only while its overlay is open.
	VideoPause        string `toml:"video_pause"`
	VideoSeekBackward string `toml:"video_seek_backward"`
	VideoSeekForward  string `toml:"video_seek_forward"`
	VideoReplay       string `toml:"video_replay"`
}

// Nitro controls how the picker sends emoji and stickers the account cannot use
// natively.
type Nitro struct {
	// Fake enables the "fake nitro" fallback: instead of a native emoji/sticker
	// send, the picker inserts the CDN URL, which tuicord renders back inline.
	Fake bool `toml:"fake"`
}

// Media controls network, decode, cache, and player resource limits. Byte and
// pixel values are direct limits so operators can audit the exact bounds.
type Media struct {
	Enabled               bool  `toml:"enabled"`
	AnimateGIFs           bool  `toml:"animate_gifs"`
	EmojiImages           bool  `toml:"emoji_images"`
	MaxHeightCells        int   `toml:"max_height_cells"`
	MaxResponseBytes      int64 `toml:"max_response_bytes"`
	MaxSourcePixels       int64 `toml:"max_source_pixels"`
	MaxSourceDimension    int   `toml:"max_source_dimension"`
	MaxGIFFrames          int   `toml:"max_gif_frames"`
	MaxGIFMemoryBytes     int64 `toml:"max_gif_memory_bytes"`
	RequestTimeoutSeconds int   `toml:"request_timeout_seconds"`
	ConcurrentFetches     int   `toml:"concurrent_fetches"`
	QueuedFetches         int   `toml:"queued_fetches"`
	DecodedCacheMaxBytes  int64 `toml:"decoded_cache_max_bytes"`
	CacheMaxBytes         int64 `toml:"cache_max_bytes"`
	CacheTTLHours         int   `toml:"cache_ttl_hours"`

	ViewerMaxResponseBytes   int64 `toml:"viewer_max_response_bytes"`
	ViewerMaxSourcePixels    int64 `toml:"viewer_max_source_pixels"`
	ViewerMaxSourceDimension int   `toml:"viewer_max_source_dimension"`
	ViewerMaxGIFFrames       int   `toml:"viewer_max_gif_frames"`
	ViewerMaxGIFMemoryBytes  int64 `toml:"viewer_max_gif_memory_bytes"`

	VideoEnabled bool   `toml:"video_enabled"`
	MpvPath      string `toml:"mpv_path"`
	VideoAudio   bool   `toml:"video_audio"`
	// VideoUseSHM is "auto", "true", or "false". Auto enables shared memory
	// only for local sessions, preserving the established SSH behavior.
	VideoUseSHM string `toml:"video_use_shm"`
}

// Privacy controls which media-related operations may touch the network, disk,
// or system clipboard. Defaults preserve existing behavior and can be disabled
// independently without disabling text chat.
type Privacy struct {
	FetchExternalMedia      bool  `toml:"fetch_external_media"`
	PersistMediaCache       bool  `toml:"persist_media_cache"`
	PrefetchMedia           bool  `toml:"prefetch_media"`
	ClipboardImages         bool  `toml:"clipboard_images"`
	ClipboardMaxBytes       int64 `toml:"clipboard_max_bytes"`
	ClipboardTimeoutSeconds int   `toml:"clipboard_timeout_seconds"`
	PlayVideos              bool  `toml:"play_videos"`
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
	// RoleGradients enables Discord gradient role colors when role metadata
	// provides gradient stops. It is off by default for conservative terminal
	// color usage.
	RoleGradients bool `toml:"role_gradients"`
	// RoleGradientAnimations phase-shifts enabled role gradients on visible
	// chat rows. It has no effect unless RoleGradients is also enabled.
	RoleGradientAnimations bool `toml:"role_gradient_animations"`
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

// Plugins controls the Lua plugin system. Plugins are loaded from the plugins/
// directory beside config.toml. They run under a restricted sandbox; extra
// capabilities (filesystem, network) are granted per-plugin via Grants.
type Plugins struct {
	// Enabled is the master switch. When false, no plugins are loaded.
	Enabled bool `toml:"enabled"`
	// Disabled lists plugin names (file base name without .lua) to skip.
	Disabled []string `toml:"disabled"`
	// Grants maps a plugin name to the capabilities the user has granted it,
	// e.g. "fs" or "net". Absent from the map means the plugin runs fully
	// sandboxed.
	Grants map[string][]string `toml:"grants"`
}

// Config is the full user configuration.
type Config struct {
	Layout        Layout        `toml:"layout"`
	Keys          Keys          `toml:"keys"`
	Colors        Colors        `toml:"colors"`
	Nitro         Nitro         `toml:"nitro"`
	Media         Media         `toml:"media"`
	Privacy       Privacy       `toml:"privacy"`
	Display       Display       `toml:"display"`
	Auth          Auth          `toml:"auth"`
	Accessibility Accessibility `toml:"accessibility"`
	Integrations  Integrations  `toml:"integrations"`
	// Plugins is held by pointer so Config stays comparable (its Disabled slice
	// and Grants map are not). A nil pointer means "plugins enabled, none
	// disabled, no grants" — see PluginsEnabled/PluginDisabled/PluginGrants.
	Plugins        *Plugins        `toml:"plugins"`
	ColorOverrides *ColorOverrides `toml:"-"`
}

// PluginsEnabled reports whether the plugin system should load plugins. A
// missing [plugins] section defaults to enabled.
func (c Config) PluginsEnabled() bool {
	return c.Plugins == nil || c.Plugins.Enabled
}

// PluginDisabled reports whether the named plugin is in the disabled list.
func (c Config) PluginDisabled(name string) bool {
	if c.Plugins == nil {
		return false
	}
	for _, d := range c.Plugins.Disabled {
		if d == name {
			return true
		}
	}
	return false
}

// PluginGrants returns the capabilities the user granted the named plugin.
func (c Config) PluginGrants(name string) []string {
	if c.Plugins == nil {
		return nil
	}
	return c.Plugins.Grants[name]
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
			QuickSwitcher:     "ctrl+k",
			Help:              "ctrl+/",
			NextPanel:         "tab",
			FocusComposer:     "esc",
			Picker:            "ctrl+e",
			PasteImage:        "ctrl+v",
			VideoPause:        "space",
			VideoSeekBackward: "left",
			VideoSeekForward:  "right",
			VideoReplay:       "r",
		},
		Nitro: Nitro{Fake: true},
		Media: Media{
			Enabled:                  true,
			AnimateGIFs:              true,
			EmojiImages:              true,
			MaxHeightCells:           12,
			MaxResponseBytes:         25 << 20,
			MaxSourcePixels:          40_000_000,
			MaxSourceDimension:       16_384,
			MaxGIFFrames:             120,
			MaxGIFMemoryBytes:        192 << 20,
			RequestTimeoutSeconds:    15,
			ConcurrentFetches:        6,
			QueuedFetches:            48,
			DecodedCacheMaxBytes:     64 << 20,
			CacheMaxBytes:            256 << 20,
			CacheTTLHours:            7 * 24,
			ViewerMaxResponseBytes:   50 << 20,
			ViewerMaxSourcePixels:    80_000_000,
			ViewerMaxSourceDimension: 24_576,
			ViewerMaxGIFFrames:       180,
			ViewerMaxGIFMemoryBytes:  384 << 20,
			VideoEnabled:             true,
			MpvPath:                  "mpv",
			VideoUseSHM:              "auto",
		},
		Privacy: Privacy{
			FetchExternalMedia:      true,
			PersistMediaCache:       true,
			PrefetchMedia:           true,
			ClipboardImages:         true,
			ClipboardMaxBytes:       25 << 20,
			ClipboardTimeoutSeconds: 5,
			PlayVideos:              true,
		},
		Accessibility: Accessibility{MouseOn: true},
		// Catppuccin Latte: the light variant of the Catppuccin palette.
		Colors: Colors{
			Enabled:    true,
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

// PluginsDir returns the directory Lua plugins are loaded from, beside
// config.toml. It honors XDG_CONFIG_HOME through the same resolution as Path.
func PluginsDir() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "plugins"), nil
}

// ConfigLuaPath returns the path to the optional Lua configuration file beside
// config.toml. It is loaded independently of the plugin system, so settings and
// keybindings can be expressed in Lua without writing a plugin.
func ConfigLuaPath() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "config.lua"), nil
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
	return atomicfile.Write(path, 0o644, func(w io.Writer) error {
		return toml.NewEncoder(w).Encode(cfg)
	})
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
		// Create an empty plugins/ directory so the location is discoverable on
		// first run. A failure here is not fatal to loading config.
		_ = os.MkdirAll(filepath.Join(filepath.Dir(path), "plugins"), 0o755)
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
	return writeFirstRunFile(path, defaultConfigTemplate)
}

func writeColorsTemplate(path string) error {
	return writeFirstRunFile(path, colorsTemplate())
}

func writeFirstRunFile(path, contents string) error {
	err := atomicfile.WriteNew(path, 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, contents)
		return err
	})
	// Another process (or the user) may create the file while the template is
	// being encoded. Atomic no-clobber installation preserves that winner.
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
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
# Attach an image from the clipboard (also available as ;paste). Set empty to
# disable; text paste (ctrl+shift+v) is unaffected either way.
paste_image = "ctrl+v"
# Video overlay controls.
video_pause = "space"
video_seek_backward = "left"
video_seek_forward = "right"
video_replay = "r"

[colors]
# Catppuccin Latte is enabled by default. Set enabled = false to ignore
# custom values in this section and use the built-in palette.
enabled = true
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
# Render cached Discord gradient roles on author names.
role_gradients = false
# Animate visible gradient role names (requires role_gradients = true).
role_gradient_animations = false
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

[media]
enabled = true
animate_gifs = true
emoji_images = true
max_height_cells = 12
max_response_bytes = 26214400
max_source_pixels = 40000000
max_source_dimension = 16384
max_gif_frames = 120
max_gif_memory_bytes = 201326592
request_timeout_seconds = 15
concurrent_fetches = 6
queued_fetches = 48
cache_max_bytes = 268435456
cache_ttl_hours = 168
viewer_max_response_bytes = 52428800
viewer_max_source_pixels = 80000000
viewer_max_source_dimension = 24576
viewer_max_gif_frames = 180
viewer_max_gif_memory_bytes = 402653184
video_enabled = true
mpv_path = "mpv"
video_audio = false
video_use_shm = "auto"

[privacy]
# Disable any of these independently to prevent the corresponding external IO.
fetch_external_media = true
persist_media_cache = true
prefetch_media = true
clipboard_images = true
clipboard_max_bytes = 26214400
clipboard_timeout_seconds = 5
play_videos = true

[integrations.slash_commands]
enabled = false

# [plugins]
# Lua plugins are loaded from the plugins/ directory beside this file.
# Plugins are enabled by default; set enabled = false to load none.
# enabled = true
# disabled = ["some-plugin"]   # skip plugins by file base name
#
# Plugins run sandboxed (no filesystem or network). Grant extra capabilities
# per-plugin here; recognized capabilities are "fs" and "net".
# [plugins.grants]
# some-plugin = ["fs"]

# Fine-grained cell colors live beside this file in colors.conf.
# Example:
# messages.author.fg=#ff00ff
# messages.link.prettyLink.fg=#0000ff
# messages.header{n}.attrs=bold|underline
# messages.*.bg=#ffffff
`
