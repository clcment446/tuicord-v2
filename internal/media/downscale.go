package media

import (
	"image"

	"golang.org/x/image/draw"
)

// cellAspect is the pixel-aspect correction for a typical terminal cell.
// A cell is 1 column wide and ~2 rows tall in terms of pixel coverage
// (half-block rendering stacks two pixels per row). Accounting for this means
// maxRows×2 effective pixel rows for the height budget.
const cellAspect = 2

// Downscale returns a new image scaled to fit within maxCols×maxRows terminal
// cells while preserving the original aspect ratio. It uses the CatmullRom
// resampler for high-quality output at any scale factor.
//
// Terminal cells are not square: each cell covers roughly twice as many
// vertical pixels as horizontal pixels when rendered with half-block characters
// (▀/▄). Downscale accounts for this by treating the height budget as
// maxRows×cellAspect effective pixel rows before computing the fit.
//
// If img already fits within the cell budget, it is returned unchanged.
// maxCols and maxRows must both be positive; zero or negative values produce
// a 1×1 placeholder.
func Downscale(img image.Image, maxCols, maxRows int) image.Image {
	if maxCols <= 0 {
		maxCols = 1
	}
	if maxRows <= 0 {
		maxRows = 1
	}

	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return img
	}

	// Convert the cell budget to pixel budget.
	// Each column maps to 1 px wide; each row maps to cellAspect px tall.
	budgetW := maxCols
	budgetH := maxRows * cellAspect

	if srcW <= budgetW && srcH <= budgetH {
		return img
	}

	// Scale down preserving aspect ratio: find the limiting dimension.
	scaleW := float64(budgetW) / float64(srcW)
	scaleH := float64(budgetH) / float64(srcH)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	dstW := max(int(float64(srcW)*scale), 1)
	dstH := max(int(float64(srcH)*scale), 1)

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
