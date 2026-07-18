//go:build !linux

package media

// VideoPlayer is a no-op on platforms without the pty-backed mpv integration.
// Its methods report that inline video is unavailable so callers fall back to a
// static poster or chip.
type VideoPlayer struct{}

// NewVideoPlayer returns a disabled player.
func NewVideoPlayer(cfg Config) *VideoPlayer { return &VideoPlayer{} }

// Available always reports false off Linux.
func (p *VideoPlayer) Available() bool { return false }

// Playing always reports no active playback.
func (p *VideoPlayer) Playing() (string, bool) { return "", false }

// Play always returns ErrVideoUnsupported.
func (p *VideoPlayer) Play(url string, region, term Rect, out func([]byte), onExit func()) error {
	return ErrVideoUnsupported
}

// Stop is a no-op.
func (p *VideoPlayer) Stop() {}
