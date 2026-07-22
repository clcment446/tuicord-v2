package ui

import (
	"image"
	"os"

	"awesomeProject/internal/media"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

const (
	// composerPreviewMaxCols/Rows bound one image thumbnail's cell footprint.
	composerPreviewMaxCols = 48
	composerPreviewMaxRows = 8
	// composerPreviewMaxTotal caps the combined height of all thumbnails so the
	// composer never eats the whole screen when several images are staged.
	composerPreviewMaxTotal = 12
	// composerBaseBasis is the composer border height (content + frame) with no
	// image preview; it grows by the preview height when images are staged.
	composerBaseBasis = 8
)

// imagePreview renders a staged image inline above the composer at a fixed cell
// footprint. It uploads the full-resolution image through the Kitty graphics
// path (with a cell fallback on terminals without graphics) and lets the
// terminal fit it into cols×rows cells, exactly like inline chat media — so the
// thumbnail is crisp rather than an upscaled low-res blur.
type imagePreview struct {
	img   image.Image
	cols  int
	rows  int
	id    uint32
	style screen.Style
	node  layout.Node
}

func (w *imagePreview) Measure(avail tui.Size) tui.Size {
	cols, rows := w.cols, w.rows
	if avail.W > 0 && cols > avail.W {
		cols = avail.W
	}
	if avail.H > 0 && rows > avail.H {
		rows = avail.H
	}
	return tui.Size{W: max(cols, 1), H: max(rows, 1)}
}

func (w *imagePreview) Layout() *layout.Node { return &w.node }

func (w *imagePreview) Draw(r screen.Region) {
	if w == nil || w.img == nil {
		return
	}
	cols := min(w.cols, r.Width())
	rows := min(w.rows, r.Height())
	img := widget.NewKittyImageFrom(w.img).SetID(w.id).SetZ(-1).SetStyle(w.style)
	if b := w.img.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
		img.SetPixelSize(b.Dx(), b.Dy())
	}
	img.Draw(r.Clip(screen.Rect{X: 0, Y: 0, W: max(cols, 1), H: max(rows, 1)}))
}

func (w *imagePreview) Handle(tui.Event) bool { return false }

// decodeImagePreview decodes an image file into a crisp, bounded thumbnail widget
// sized for the composer. It downscales to the exact display pixel budget so the
// terminal renders it 1:1 rather than upscaling a tiny image. It is pure (no
// MainView access) so it can run off the UI goroutine.
func decodeImagePreview(path string, cellW, cellH int, style screen.Style) (*imagePreview, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, err := media.Decode(f)
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	cols, rows := fitMediaCells(b.Dx(), b.Dy(), composerPreviewMaxCols, composerPreviewMaxRows)
	scaled := media.DownscaleToPixels(img, cols*cellW, rows*cellH)
	p := &imagePreview{
		img:   scaled,
		cols:  cols,
		rows:  rows,
		id:    stableImageID("composer-preview:" + path),
		style: style,
	}
	p.node.Basis = rows
	return p, true
}

// requestImagePreview decodes and downscales one staged image off the UI thread,
// then posts the finished thumbnail back and rebuilds the preview. Large images
// previously decoded on the UI goroutine, stalling input while several were
// staged. syncPreviewDecode makes the decode inline for tests that have no event
// loop to drain the posted result.
func (mv *MainView) requestImagePreview(path string) {
	if mv == nil {
		return
	}
	if mv.previewCache == nil {
		mv.previewCache = map[string]*imagePreview{}
	}
	if mv.previewPending == nil {
		mv.previewPending = map[string]bool{}
	}
	cellW, cellH := mv.previewCellW, mv.previewCellH
	style := mv.styles.Cell("messages.content")
	if mv.syncPreviewDecode {
		if p, ok := decodeImagePreview(path, cellW, cellH, style); ok {
			mv.previewCache[path] = p
		}
		return
	}
	if mv.app == nil || mv.previewPending[path] {
		return
	}
	mv.previewPending[path] = true
	go func() {
		p, ok := decodeImagePreview(path, cellW, cellH, style)
		mv.app.Post(func() {
			delete(mv.previewPending, path)
			// The attachment may have been unstaged (sent/cleared) while decoding.
			if !ok || !mv.isStagedImage(path) {
				return
			}
			mv.previewCache[path] = p
			mv.updateComposerPreview()
		})
	}()
}

// isStagedImage reports whether path is still a staged image attachment.
func (mv *MainView) isStagedImage(path string) bool {
	for _, a := range mv.attachments {
		if a.path == path && isImageFilename(a.meta.Filename) {
			return true
		}
	}
	return false
}

// updateComposerPreview rebuilds the inline image thumbnails above the composer
// from the staged image attachments and adjusts the composer's height to fit.
// Thumbnails not yet decoded are requested off the UI thread and appear once
// their decode posts back.
func (mv *MainView) updateComposerPreview() {
	if mv.composerPreview == nil {
		return
	}
	var rows []tui.Widget
	total := 0
	for _, attachment := range mv.imageAttachments() {
		p, ok := mv.previewCache[attachment.path]
		if !ok {
			// Kick off the decode; async path shows it on a later rebuild, the sync
			// (test) path populates the cache immediately.
			mv.requestImagePreview(attachment.path)
			p, ok = mv.previewCache[attachment.path]
		}
		if !ok {
			continue
		}
		rows = append(rows, p)
		total += p.rows
		if total >= composerPreviewMaxTotal {
			break
		}
	}
	mv.composerPreview.SetChildren(rows...)

	node := mv.composerPreview.Layout()
	node.Grow = 0
	if len(rows) == 0 {
		node.Hidden = true
		node.Basis = 0
	} else {
		node.Hidden = false
		node.Basis = min(total, composerPreviewMaxTotal)
	}

	if mv.composerNode != nil {
		if len(rows) == 0 {
			mv.composerNode.Basis = composerBaseBasis
		} else {
			mv.composerNode.Basis = composerBaseBasis + min(total, composerPreviewMaxTotal)
		}
	}
}
