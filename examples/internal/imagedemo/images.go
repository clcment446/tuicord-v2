package imagedemo

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
)

// LocalImagePath returns a useful local image when one exists.
func LocalImagePath() string {
	if path := os.Getenv("TUI_IMAGE_PATH"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, rel := range []string{
		"Pictures/avatar.jpg",
		"Pictures/yixuan_bg_1.png",
		"Pictures/absolute_cinema.png",
		"Pictures/bg_win_1.jpg",
	} {
		path := filepath.Join(home, rel)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// Gradient returns a generated color image for deterministic examples.
func Gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(40 + 180*x/maxInt(w-1, 1))
			g := uint8(60 + 150*y/maxInt(h-1, 1))
			b := uint8(190 - 110*x/maxInt(w-1, 1) + 40*y/maxInt(h-1, 1))
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// Radial returns a generated grayscale image that makes scaling artifacts easy
// to see.
func Radial(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2
	maxDist := math.Hypot(cx, cy)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / maxDist
			v := uint8(235 - 210*math.Min(d, 1))
			if (x/8+y/6)%2 == 0 {
				v = uint8(float64(v) * 0.72)
			}
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// Checker returns a generated fallback image.
func Checker(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/12+y/8)%2 == 0 {
				img.SetRGBA(x, y, color.RGBA{R: 85, G: 115, B: 170, A: 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{R: 28, G: 32, B: 42, A: 255})
			}
		}
	}
	return img
}

// Protocol returns a generated image with a high-contrast center shape.
func Protocol(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(220 * x / maxInt(w-1, 1))
			g := uint8(220 * y / maxInt(h-1, 1))
			b := uint8(70 + 130*(w-x)/maxInt(w, 1))
			if (x-w/2)*(x-w/2)+(y-h/2)*(y-h/2) < (h/4)*(h/4) {
				r, g, b = 255, 240, 150
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
