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

	// CellPixelWidth and CellPixelHeight are the pixel dimensions of one
	// terminal cell, used to convert a cell budget into the pixel budget that
	// Kitty graphics needs. Zero means the terminal did not report a size; use
	// CellPixels for the defaulted values rather than reading these directly.
	CellPixelWidth  int
	CellPixelHeight int
}

// Default cell pixel size, used when the terminal does not report one. Chosen
// to match a typical monospace cell at common font sizes; being slightly off
// costs a little image quality, never correctness.
const (
	defaultCellPixelWidth  = 10
	defaultCellPixelHeight = 20
)

// CellPixels returns the configured cell size in pixels, substituting
// conventional defaults for unreported (zero) values.
func (c Config) CellPixels() (w, h int) {
	w, h = c.CellPixelWidth, c.CellPixelHeight
	if w <= 0 {
		w = defaultCellPixelWidth
	}
	if h <= 0 {
		h = defaultCellPixelHeight
	}
	return w, h
}

// DefaultConfig returns a Config with sensible defaults for a first-run
// experience. Media is enabled, animations play, custom emoji render inline,
// and the system's default URI handler opens videos.
func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		MaxHeightCells: 12,
		Animate:        true,
		EmojiImages:    true,
		VideoPlayer:    "xdg-open",
	}
}
