package media

import (
	"errors"
	"strconv"
)

// ErrVideoUnsupported is returned by a VideoPlayer on platforms that cannot host
// an inline mpv pty session.
var ErrVideoUnsupported = errors.New("media: inline video is not supported on this platform")

// Rect is a cell-space rectangle on the terminal, addressed from a 0-based
// origin at the top-left of the screen. It is the region an inline video renders
// into.
type Rect struct {
	X, Y       int
	Cols, Rows int
}

// Empty reports whether the rectangle has no area.
func (r Rect) Empty() bool { return r.Cols <= 0 || r.Rows <= 0 }

// KittyDeleteAllImages returns terminal bytes that delete every Kitty graphics
// placement and free its image data. It erases mpv's full-screen frame in one
// escape; the caller must force a full repaint afterward so the widget tree's
// own images (which this also removes) are re-uploaded and re-placed.
func KittyDeleteAllImages() []byte {
	return []byte("\x1b_Ga=d,d=A\x1b\\")
}

// KittyClearRegion returns terminal bytes that delete every Kitty graphics
// placement intersecting r's cells. mpv (run with --vo-kitty-alt-screen=no)
// leaves its final frame on screen after it exits, and that placement is not
// part of our own frame diff, so it must be removed explicitly when playback
// stops. Deleting by cell (d=p, 1-based coordinates) targets only the video
// region and leaves inline images elsewhere untouched.
func KittyClearRegion(r Rect) []byte {
	if r.Empty() {
		return nil
	}
	var b []byte
	for row := 0; row < r.Rows; row++ {
		for col := 0; col < r.Cols; col++ {
			b = append(b, "\x1b_Ga=d,d=p,x="...)
			b = strconv.AppendInt(b, int64(r.X+col+1), 10)
			b = append(b, ",y="...)
			b = strconv.AppendInt(b, int64(r.Y+row+1), 10)
			b = append(b, "\x1b\\"...)
		}
	}
	return b
}
