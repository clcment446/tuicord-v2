package media

import "time"

const (
	DefaultMaxResponseBytes   int64 = 25 << 20
	DefaultMaxSourcePixels    int64 = 40_000_000
	DefaultMaxSourceDimension       = 16_384
	DefaultGIFMaxFrames             = 120
	DefaultGIFMaxMemoryBytes  int64 = 192 << 20
	DefaultRequestTimeout           = 15 * time.Second
	DefaultConcurrentFetches        = 6
	DefaultQueuedFetches            = 48
	MaxConcurrentFetches            = 32
	MaxQueuedFetches                = 1024
	DefaultDiskCacheMaxBytes  int64 = 256 << 20
	DefaultDiskCacheTTL             = 7 * 24 * time.Hour
)

// Config holds the user-facing settings for the media subsystem.
// The application is responsible for reading these from its configuration
// source and passing them to the relevant constructors at startup.
type Config struct {
	// Enabled controls whether media is fetched and displayed at all.
	Enabled bool

	// MaxHeightCells is the maximum number of terminal rows an inline media
	// block may occupy. The aspect ratio is always preserved.
	MaxHeightCells int

	// Animate enables multi-frame GIF playback in the chat renderer.
	Animate bool

	// EmojiImages renders custom Discord emoji as small inline images.
	EmojiImages bool

	// Network and decode limits apply before expensive image allocation or GIF
	// frame composition. Zero values are replaced by the bounded defaults above.
	MaxResponseBytes   int64
	MaxSourcePixels    int64
	MaxSourceDimension int
	GIFMaxFrames       int
	GIFMaxMemoryBytes  int64
	RequestTimeout     time.Duration
	ConcurrentFetches  int
	QueuedFetches      int

	// Disk cache policy. DiskCacheEnabled is explicit because privacy settings
	// may disable persistent media while retaining the in-memory decoded LRU.
	DiskCacheEnabled  bool
	DiskCacheMaxBytes int64
	DiskCacheTTL      time.Duration

	// Prefetch controls idle emoji/sticker cache warming.
	Prefetch bool

	// MpvPath is the mpv binary used to play videos via its Kitty output.
	MpvPath      string
	VideoEnabled bool
	VideoAudio   bool
	VideoUseSHM  bool

	// CellPixelWidth and CellPixelHeight convert cell budgets to pixels.
	CellPixelWidth  int
	CellPixelHeight int
}

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

// Bounded fills zero/negative resource fields with safe defaults.
func (c Config) Bounded() Config {
	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if c.MaxSourcePixels <= 0 {
		c.MaxSourcePixels = DefaultMaxSourcePixels
	}
	if c.MaxSourceDimension <= 0 {
		c.MaxSourceDimension = DefaultMaxSourceDimension
	}
	if c.GIFMaxFrames <= 0 {
		c.GIFMaxFrames = DefaultGIFMaxFrames
	}
	if c.GIFMaxMemoryBytes <= 0 {
		c.GIFMaxMemoryBytes = DefaultGIFMaxMemoryBytes
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.ConcurrentFetches <= 0 {
		c.ConcurrentFetches = DefaultConcurrentFetches
	} else if c.ConcurrentFetches > MaxConcurrentFetches {
		c.ConcurrentFetches = MaxConcurrentFetches
	}
	if c.QueuedFetches <= 0 {
		c.QueuedFetches = DefaultQueuedFetches
	} else if c.QueuedFetches > MaxQueuedFetches {
		c.QueuedFetches = MaxQueuedFetches
	}
	if c.DiskCacheMaxBytes <= 0 {
		c.DiskCacheMaxBytes = DefaultDiskCacheMaxBytes
	}
	if c.DiskCacheTTL <= 0 {
		c.DiskCacheTTL = DefaultDiskCacheTTL
	}
	return c
}

// DefaultConfig returns a Config with bounded, privacy-conscious defaults while
// retaining the existing visible media, animation, prefetch, and video behavior.
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		MaxHeightCells:     12,
		Animate:            true,
		EmojiImages:        true,
		MaxResponseBytes:   DefaultMaxResponseBytes,
		MaxSourcePixels:    DefaultMaxSourcePixels,
		MaxSourceDimension: DefaultMaxSourceDimension,
		GIFMaxFrames:       DefaultGIFMaxFrames,
		GIFMaxMemoryBytes:  DefaultGIFMaxMemoryBytes,
		RequestTimeout:     DefaultRequestTimeout,
		ConcurrentFetches:  DefaultConcurrentFetches,
		QueuedFetches:      DefaultQueuedFetches,
		DiskCacheEnabled:   true,
		DiskCacheMaxBytes:  DefaultDiskCacheMaxBytes,
		DiskCacheTTL:       DefaultDiskCacheTTL,
		Prefetch:           true,
		MpvPath:            "mpv",
		VideoEnabled:       true,
	}
}
