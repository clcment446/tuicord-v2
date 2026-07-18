package ui

import (
	"image"
	"strings"
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
	styles         Styles
	node           layout.Node
	title          string
	key            string
	img            image.Image // nil for video
	frames         []media.Frame
	frame          int
	last           time.Time
	elapsed        time.Duration
	onClose        func()
	video          bool
	status         media.VideoStatus
	onToggle       func()
	onReplay       func()
	onSeek         func(float64)
	onSeekRelative func(float64)
	readStatus     func() (media.VideoStatus, error)
	pauseKey       string
	seekBackKey    string
	seekForwardKey string
	replayKey      string
	width, height  int
	onResize       func(int, int)
}

const videoControlRows = 2
const mediaViewerPadding = 1

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

func (v *mediaViewer) setImage(img image.Image) {
	if v == nil || img == nil {
		return
	}
	v.img = img
	v.frames = nil
	v.frame = 0
	v.last = time.Time{}
	v.elapsed = 0
}

func (v *mediaViewer) setFrames(frames []media.Frame) {
	if v == nil || len(frames) == 0 {
		return
	}
	v.frames = append([]media.Frame(nil), frames...)
	v.frame = 0
	v.img = v.frames[0].Image
	v.last = time.Time{}
	v.elapsed = 0
}

func (v *mediaViewer) setVideoControls(toggle, replay func(), seek func(float64), status func() (media.VideoStatus, error)) {
	v.video = true
	v.onToggle = toggle
	v.onReplay = replay
	v.onSeek = seek
	v.readStatus = status
}

func (v *mediaViewer) setVideoKeys(pause, seekBack, seekForward, replay string, seekRelative func(float64)) {
	v.pauseKey = pause
	v.seekBackKey = seekBack
	v.seekForwardKey = seekForward
	v.replayKey = replay
	v.onSeekRelative = seekRelative
}

func (v *mediaViewer) setVideoResize(resize func(int, int)) { v.onResize = resize }

// Draw fills the backdrop, centers the image (when present), and prints a hint.
func (v *mediaViewer) Draw(r screen.Region) {
	bg := v.styles.Cell("mediaviewer.background")
	if !bg.Bg.Set() {
		bg = screen.Style{Bg: screen.RGB(0, 0, 0)}
	}
	fill(r, bg)

	w, h := r.Width(), r.Height()
	resized := v.video && v.width > 0 && v.height > 0 && (v.width != w || v.height != h)
	v.width, v.height = w, h
	if resized && v.onResize != nil {
		v.onResize(w, h)
	}
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
	if v.video {
		v.drawVideoControls(r, w, h, bg, hintStyle)
		return
	}
	if hint == "" {
		hint = "Esc to close"
	}
	drawViewerText(r, max((w-text.Width(hint))/2, 0), h-1, hint, hintStyle)
}

func (v *mediaViewer) drawVideoControls(r screen.Region, w, h int, bg, style screen.Style) {
	if h < 2 || w <= mediaViewerPadding*2 {
		return
	}
	x := mediaViewerPadding
	innerW := w - mediaViewerPadding*2
	button := "Ⅱ"
	if v.status.Paused || v.status.Ended {
		button = "▶"
	}
	controls := button + "  ↺  "
	barX := text.Width(controls)
	barW := max(innerW-barX, 1)
	progress := 0.0
	if v.status.Duration > 0 {
		progress = v.status.Position / v.status.Duration
	}
	progress = max(0.0, min(progress, 1.0))
	filled := int(progress * float64(barW))
	bar := strings.Repeat("━", filled) + strings.Repeat("─", max(barW-filled, 0))
	drawViewerText(r, x, h-2, controls+bar, style)
	pauseKey := v.pauseKey
	if pauseKey == "" {
		pauseKey = "click"
	}
	replayKey := v.replayKey
	if replayKey == "" {
		replayKey = "click"
	}
	seekKeys := v.seekBackKey + "/" + v.seekForwardKey
	if v.seekBackKey == "" || v.seekForwardKey == "" {
		seekKeys = "click"
	}
	hint := pauseKey + " pause/play · " + replayKey + " replay · " + seekKeys + " seek · Esc close"
	drawViewerText(r, max((w-text.Width(hint))/2, 0), h-1, hint, style)
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
		changed := v.advance(time.Now())
		if v.video && v.readStatus != nil {
			if status, err := v.readStatus(); err == nil && status != v.status {
				v.status = status
				changed = true
			}
		}
		return changed
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		if v.video {
			switch {
			case keyMatches(ev, v.pauseKey):
				if v.status.Ended && v.onReplay != nil {
					v.onReplay()
				} else if v.onToggle != nil {
					v.onToggle()
				}
				return true
			case keyMatches(ev, v.seekBackKey):
				if v.onSeekRelative != nil {
					v.onSeekRelative(-5)
				}
				return true
			case keyMatches(ev, v.seekForwardKey):
				if v.onSeekRelative != nil {
					v.onSeekRelative(5)
				}
				return true
			case keyMatches(ev, v.replayKey):
				if v.onReplay != nil {
					v.onReplay()
				}
				return true
			}
		}
		if ev.Key == input.KeyEsc || (ev.Key == input.KeyRune && (ev.Rune == 'q' || ev.Rune == 'o')) {
			if v.onClose != nil {
				v.onClose()
			}
			return true
		}
	case input.MouseEvent:
		if ev.Kind == input.MousePress {
			if v.video {
				if ev.Y != v.height-2 {
					return true
				}
				x := ev.X - mediaViewerPadding
				innerW := v.width - mediaViewerPadding*2
				if x < 0 || x >= innerW {
					return true
				}
				switch {
				case x < 3:
					if v.status.Ended && v.onReplay != nil {
						v.onReplay()
					} else if v.onToggle != nil {
						v.onToggle()
					}
				case x < 6:
					if v.onReplay != nil {
						v.onReplay()
					}
				default:
					if v.onSeek != nil && innerW > 6 {
						v.onSeek(float64(x-6) * 100 / float64(innerW-6))
					}
				}
				return true
			}
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
