package media

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"time"

	// Register standard decoders so image.Decode recognises PNG, JPEG, and GIF
	// out of the box when the caller has not imported these packages themselves.
	_ "image/jpeg"
	_ "image/png"

	// Register WebP decoder from the extended image library.
	_ "golang.org/x/image/webp"
)

// Frame holds one fully composed frame of an animated GIF.
type Frame struct {
	// Image is the complete frame with all previous frames composited in,
	// ready to pass to widget.NewImageFrom.
	Image image.Image
	// Delay is the inter-frame pause the player should wait before advancing
	// to the next frame. GIF stores delay in centiseconds; this field converts
	// that to a Go Duration so callers need not know the encoding.
	Delay time.Duration
}

// Decode decodes an image from r. The format is detected automatically;
// supported formats are PNG, JPEG, GIF (first frame only), and WebP.
// For animated GIFs use DecodeGIF to obtain all frames.
func Decode(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("media: decode: %w", err)
	}
	return img, nil
}

// DecodeGIF decodes all frames of an animated GIF from r. Each returned Frame
// contains the fully composed image (prior frames are blended according to the
// GIF disposal method, so every Frame.Image is a complete picture) plus the
// delay the player should observe before showing the next frame.
//
// Disposal method 0/1 (do-not-dispose): the previous canvas is left in place
// and the new frame is drawn on top.
// Disposal method 2 (restore-to-background): the prior frame's region is
// cleared to the background colour before the next frame is drawn.
// Disposal method 3 (restore-to-previous): treated as do-not-dispose for
// simplicity (correct for the vast majority of Discord GIFs).
func DecodeGIF(r io.Reader) ([]Frame, error) {
	g, err := gif.DecodeAll(r)
	if err != nil {
		return nil, fmt.Errorf("media: decode gif: %w", err)
	}
	if len(g.Image) == 0 {
		return nil, fmt.Errorf("media: decode gif: no frames")
	}

	width := g.Config.Width
	height := g.Config.Height
	if width == 0 || height == 0 {
		// Fall back to first frame bounds when the config header is absent.
		b := g.Image[0].Bounds()
		width = b.Max.X
		height = b.Max.Y
	}

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill canvas with the GIF background colour when a palette is present.
	if g.Config.ColorModel != nil {
		if idx, ok := g.BackgroundIndex, true; ok && int(idx) < 256 {
			if p, ok := g.Config.ColorModel.(color.Palette); ok && int(idx) < len(p) {
				draw.Draw(canvas, canvas.Bounds(), &image.Uniform{p[idx]}, image.Point{}, draw.Src)
			}
		}
	}

	frames := make([]Frame, 0, len(g.Image))

	for i, src := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		delay := time.Duration(0)
		if i < len(g.Delay) {
			delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}

		// Draw this frame onto the canvas.
		draw.Draw(canvas, src.Bounds(), src, src.Bounds().Min, draw.Over)

		// Snapshot the current canvas as the composed frame.
		snapshot := image.NewRGBA(canvas.Bounds())
		draw.Draw(snapshot, snapshot.Bounds(), canvas, image.Point{}, draw.Src)

		frames = append(frames, Frame{Image: snapshot, Delay: delay})

		// Apply disposal for the NEXT iteration.
		switch disposal {
		case 2: // restore-to-background
			bg := color.RGBA{}
			if g.Config.ColorModel != nil {
				if p, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(p) {
					r32, gv, b32, a32 := p[g.BackgroundIndex].RGBA()
					bg = color.RGBA{
						R: uint8(r32 >> 8),
						G: uint8(gv >> 8),
						B: uint8(b32 >> 8),
						A: uint8(a32 >> 8),
					}
				}
			}
			draw.Draw(canvas, src.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
			// 0, 1, 3: leave canvas as-is.
		}
	}

	return frames, nil
}
