package media

import (
	"bytes"
	"errors"
	"strconv"
)

// VideoStatus is the player state displayed by the video overlay controls.
type VideoStatus struct {
	Position float64
	Duration float64
	Paused   bool
	Ended    bool
}

// kittyOutputFramer turns arbitrarily split PTY reads into atomic terminal
// writes. A Kitty image is commonly transmitted as many APC commands with
// m=1 continuation flags. No unrelated cell diff may appear between them or
// the remaining base64 payload can be rendered as ordinary terminal text.
type kittyOutputFramer struct {
	pending      []byte
	transmission []byte
}

var (
	kittyAPCStart = []byte("\x1b_G")
	kittyAPCEnd   = []byte("\x1b\\")
)

func (f *kittyOutputFramer) Push(data []byte) [][]byte {
	if len(data) > 0 {
		f.pending = append(f.pending, data...)
	}
	var output [][]byte
	for len(f.pending) > 0 {
		start := bytes.Index(f.pending, kittyAPCStart)
		if start < 0 {
			// Retain enough trailing bytes for an APC prefix split across reads.
			keep := min(len(f.pending), len(kittyAPCStart)-1)
			if emit := len(f.pending) - keep; emit > 0 {
				chunk := append([]byte(nil), f.pending[:emit]...)
				if len(f.transmission) > 0 {
					f.transmission = append(f.transmission, chunk...)
				} else {
					output = append(output, chunk)
				}
			}
			f.pending = append([]byte(nil), f.pending[len(f.pending)-keep:]...)
			break
		}
		if start > 0 {
			prefix := append([]byte(nil), f.pending[:start]...)
			f.pending = f.pending[start:]
			if len(f.transmission) > 0 {
				f.transmission = append(f.transmission, prefix...)
			} else {
				output = append(output, prefix)
			}
			continue
		}
		end := bytes.Index(f.pending[len(kittyAPCStart):], kittyAPCEnd)
		if end < 0 {
			break
		}
		end += len(kittyAPCStart) + len(kittyAPCEnd)
		packet := append([]byte(nil), f.pending[:end]...)
		f.pending = f.pending[end:]
		headerEnd := bytes.IndexByte(packet, ';')
		header := packet
		if headerEnd >= 0 {
			header = packet[:headerEnd]
		}
		// For streamed payloads m=1 means more APC chunks follow. mpv also sets
		// m=1 on shared-memory (t=s) commands, but those carry only a complete
		// object name and must be forwarded immediately as one frame.
		sharedMemory := bytes.Contains(header, []byte("t=s"))
		continues := !sharedMemory && bytes.Contains(header, []byte("m=1"))
		if len(f.transmission) > 0 || continues {
			f.transmission = append(f.transmission, packet...)
			if !continues {
				output = append(output, append([]byte(nil), f.transmission...))
				f.transmission = f.transmission[:0]
			}
		} else {
			output = append(output, packet)
		}
	}
	return output
}

func (f *kittyOutputFramer) Flush() []byte {
	out := append([]byte(nil), f.transmission...)
	out = append(out, f.pending...)
	f.transmission = nil
	f.pending = nil
	return out
}

// ErrVideoUnsupported is returned by a VideoPlayer on platforms that cannot host
// an inline mpv pty session.
var ErrVideoUnsupported = errors.New("media: inline video is not supported on this platform")

// ErrVideoFrameTooLarge reports an mpv shared-memory frame that exceeds the
// configured media byte limit. Callers drop the complete Kitty APC command so
// no partial graphics payload can leak into the terminal stream.
var ErrVideoFrameTooLarge = errors.New("media: mpv shared-memory frame exceeds the configured size limit")

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
