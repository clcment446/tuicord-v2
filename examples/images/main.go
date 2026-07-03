package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"awesomeProject/examples/internal/demo"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := build(cancel)
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 84, H: 24})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	preview := widget.NewImageFrom(gradientImage(96, 64))
	preview.Layout().Min = 18

	kitty := widget.NewKittyImageFrom(gradientImage(240, 160))
	kitty.Layout().Min = 18

	ascii := widget.NewImageFrom(radialImage(96, 64))
	ascii.SetMode(widget.ImageASCII)
	ascii.SetStyle(screen.Style{Fg: screen.RGB(230, 230, 230), Bg: screen.RGB(18, 20, 24)})
	ascii.Layout().Min = 18

	local := localImageWidget()
	local.Layout().Min = 18

	top := widget.Row(
		titled("Unicode fallback", preview),
		titled("Kitty widget", kitty),
		titled("ASCII fallback", ascii),
		titled("File-backed image", local),
	).SetGap(1)
	top.Layout().Grow = 1

	notes := widget.NewMarkup(
		"Images are widgets, so they can sit in rows, borders, and viewports. " +
			"Use __Unicode__ for colored half blocks, ~~ASCII~~ for plain terminals, " +
			"or NewKittyImage/NewKittyImageFrom for high-resolution Kitty graphics.\n\n" +
			"Try arrows or the mouse wheel: this lower pane is a viewport containing markup text.",
	)
	notes.SetStyle(screen.Style{Fg: screen.RGB(210, 215, 225)})
	notes.SetLinkStyle(screen.Style{Fg: screen.RGB(120, 190, 255), Attrs: screen.Underline})
	view := widget.NewViewport()
	view.SetChild(notes)
	view.Layout().Grow = 1

	notesBox := titled("Notes", view)
	notesBox.Layout().Basis = 9
	notesBox.Layout().Grow = 0

	root := demo.Column(top, notesBox).WithCancel(cancel)
	top.Layout().Grow = 1
	return root
}

func titled(title string, child tui.Widget) *widget.Border {
	box := widget.NewBorder(child)
	box.SetTitle(title)
	box.SetStyle(screen.Style{Fg: screen.RGB(120, 160, 220), Attrs: screen.Bold})
	return box
}

func localImageWidget() *widget.Image {
	if path := os.Getenv("TUI_IMAGE_PATH"); path != "" {
		return widget.NewImage(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return widget.NewImageFrom(checkerImage(96, 64))
	}
	for _, rel := range []string{
		"Pictures/avatar.jpg",
		"Pictures/yixuan_bg_1.png",
		"Pictures/absolute_cinema.png",
		"Pictures/bg_win_1.jpg",
	} {
		path := filepath.Join(home, rel)
		if _, err := os.Stat(path); err == nil {
			return widget.NewImage(path)
		}
	}
	return widget.NewImageFrom(checkerImage(96, 64))
}

func gradientImage(w, h int) image.Image {
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

func radialImage(w, h int) image.Image {
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

func checkerImage(w, h int) image.Image {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
