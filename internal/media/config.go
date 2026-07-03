package media

// Config holds the user-facing settings for the media subsystem.
// The application is responsible for reading these from its configuration
// source and passing them to the relevant constructors at startup.
type Config struct {
	// Enabled controls whether media is fetched and displayed at all.
	// When false the package produces text-chip placeholders only and makes
	// no network requests.
	Enabled bool

	// MaxHeightCells is the maximum number of terminal rows an inline media
	// block may occupy. Images taller than this are downscaled to fit.
	// The aspect ratio is always preserved.
	MaxHeightCells int

	// Animate enables GIF animation via the Kitty graphics animation protocol.
	// When false the first frame is shown with a "[GIF]" badge overlay.
	Animate bool

	// EmojiImages renders custom Discord emoji as small inline images (one
	// cell, two columns) via the Kitty graphics protocol when true.
	// When false custom emoji are rendered as the text form :name:.
	EmojiImages bool

	// VideoPlayer is the command used to open video attachments when the user
	// activates an inline video block (e.g. by clicking it). The full URL is
	// appended as the last argument. Common values: "xdg-open", "mpv", "vlc".
	VideoPlayer string
}

// DefaultConfig returns a Config with sensible defaults for a first-run
// experience. Media is enabled, animations play, emoji are text-only (safe
// for all terminals), and the system's default URI handler opens videos.
func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		MaxHeightCells: 12,
		Animate:        true,
		EmojiImages:    false,
		VideoPlayer:    "xdg-open",
	}
}
