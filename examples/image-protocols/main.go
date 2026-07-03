package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"

	"awesomeProject/examples/internal/demo"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func main() {
	mode := strings.ToLower(firstNonEmpty(os.Getenv("TUI_IMAGE_PROTOCOL"), arg(1), "kitty"))
	width := atoiDefault(firstNonEmpty(os.Getenv("TUI_IMAGE_WIDTH"), arg(2)), 320)
	height := atoiDefault(firstNonEmpty(os.Getenv("TUI_IMAGE_HEIGHT"), arg(3)), 220)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root, ok := build(cancel, mode, width, height)
	if !ok {
		fmt.Fprintf(os.Stderr, "usage: %s [kitty|sixel] [width] [height]\n", os.Args[0])
		os.Exit(2)
	}
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 54, H: 18})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc, mode string, width, height int) (tui.Widget, bool) {
	img := widget.NewImageFrom(protocolImage(96, 60))
	switch mode {
	case "kitty":
		img.SetMode(widget.ImageKitty)
	case "sixel":
		img.SetMode(widget.ImageSixel)
	default:
		return nil, false
	}
	img.SetPixelSize(width, height)

	box := widget.NewBorder(img)
	box.SetTitle(fmt.Sprintf("%s payload: %dx%d px", mode, width, height))
	box.SetStyle(screen.Style{Fg: screen.RGB(120, 180, 255), Attrs: screen.Bold})
	return demo.Column(box).WithCancel(cancel), true
}

func protocolImage(w, h int) image.Image {
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

func arg(i int) string {
	if i >= len(os.Args) {
		return ""
	}
	return os.Args[i]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
