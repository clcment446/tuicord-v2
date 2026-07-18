package ui

import (
	"image"
	"time"

	"awesomeProject/internal/media"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// mediaViewer is a full-screen overlay that shows one piece of media enlarged: a
// centered image/GIF frame drawn with the Kitty protocol, or a blank backdrop
// for video (whose frames mpv paints over the whole screen). It fills the screen
// behind the media so the chat is hidden while viewing. Esc/q or a click closes
// it (dismissal is handled by the Shell overlay layer).
type mediaViewer struct {
	styles  Styles
	node    layout.Node
	title   string
	key     string
	img     image.Image // nil for video
	frames  []media.Frame
	frame   int
	last    time.Time
	elapsed time.Duration
	onClose func()
}

func newMediaViewer(styles Styles, title, key string, img image.Image, frames []media.Frame, onClose func()) *mediaViewer {
	v := &mediaViewer{styles: styles, title: title, key: key, img: img, frames: append([]media.Frame(nil), frames...), onClose: onClose, node: layout.Node{Grow: 1}}
	if len(v.frames) > 1 {
		v.img = v.frames[0].Image
	}
	return v
}

// Measure fills the available area.
func (v *mediaViewer) Measure(avail tui.Size) tui.Size { return avail }

// Layout returns the viewer node.
func (v *mediaViewer) Layout() *layout.Node { return &v.node }

// CanFocus lets the viewer receive keys.
func (v *mediaViewer) CanFocus() bool { return true }

// Animating requests fast runtime ticks while an animated GIF is open.
func (v *mediaViewer) Animating() bool { return v != nil && len(v.frames) > 1 }

// Draw fills the backdrop, centers the image (when present), and prints a hint.
func (v *mediaViewer) Draw(r screen.Region) {
	bg := v.styles.Cell("mediaviewer.background")
	if !bg.Bg.Set() {
		bg = screen.Style{Bg: screen.RGB(0, 0, 0)}
	}
	fill(r, bg)

	w, h := r.Width(), r.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if v.img != nil {
		iw, ih := v.img.Bounds().Dx(), v.img.Bounds().Dy()
		if iw > 0 && ih > 0 {
			cols, rows := fitMediaCells(iw, ih, w, max(h-1, 1))
			x := max((w-cols)/2, 0)
			y := max((max(h-1, 1)-rows)/2, 0)
			img := widget.NewKittyImageFrom(v.img).
				SetID(stableImageID("mediaviewer:"+v.key)).
				SetZ(-1).
				SetStyle(bg).
				SetPixelSize(iw, ih)
			img.Draw(r.Clip(screen.Rect{X: x, Y: y, W: cols, H: rows}))
		}
	}

	hintStyle := v.styles.Cell("mediaviewer.hint")
	if !hintStyle.Fg.Set() {
		hintStyle = mergeStyle(bg, screen.Style{Fg: screen.RGB(180, 180, 180)})
	}
	hint := v.title
	if hint == "" {
		hint = "Esc to close"
	}
	drawViewerText(r, max((w-text.Width(hint))/2, 0), h-1, hint, hintStyle)
}

func drawViewerText(r screen.Region, x, y int, s string, style screen.Style) {
	col := x
	for cluster := range text.Clusters(s) {
		if cluster.Width == 0 {
			continue
		}
		r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
		col += cluster.Width
	}
}

// Handle closes the viewer on q or a click (Esc is handled by the Shell).
func (v *mediaViewer) Handle(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.TickEvent:
		return v.advance(time.Now())
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		if ev.Key == input.KeyRune && (ev.Rune == 'q' || ev.Rune == 'o') {
			if v.onClose != nil {
				v.onClose()
			}
			return true
		}
	case input.MouseEvent:
		if ev.Kind == input.MousePress {
			if v.onClose != nil {
				v.onClose()
			}
			return true
		}
	}
	return false
}

func (v *mediaViewer) advance(now time.Time) bool {
	if v == nil || len(v.frames) < 2 {
		return false
	}
	if v.last.IsZero() {
		v.last = now
		return false
	}
	delta := now.Sub(v.last)
	v.last = now
	if delta <= 0 || delta > time.Second {
		v.elapsed = 0
		return false
	}
	v.elapsed += delta
	changed := false
	for steps := 0; steps < len(v.frames) && v.elapsed >= viewerFrameDelay(v.frames[v.frame]); steps++ {
		v.elapsed -= viewerFrameDelay(v.frames[v.frame])
		v.frame = (v.frame + 1) % len(v.frames)
		v.img = v.frames[v.frame].Image
		changed = true
	}
	return changed
}

func viewerFrameDelay(frame media.Frame) time.Duration {
	if frame.Delay <= 0 {
		return 100 * time.Millisecond
	}
	return frame.Delay
}
