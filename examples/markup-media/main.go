package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"

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
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 76, H: 22})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	message := widget.NewMarkup(
		"mira: [project notes](https://example.invalid) and [files](./attachements/tmp.log)\n" +
			"ren: ***bold** italic* mixed with __underlined__ and ~~strikethrough~~ text\n" +
			"sam: ![gradient preview](./pic.png) stays represented as a styled attachment span",
	)
	message.SetStyle(screen.Style{Fg: screen.RGB(215, 220, 230)})
	message.SetBoldStyle(screen.Style{Fg: screen.RGB(255, 235, 160), Attrs: screen.Bold})
	message.SetLinkStyle(screen.Style{Fg: screen.RGB(120, 190, 255), Attrs: screen.Underline})
	message.Layout().Basis = 5
	message.Layout().Grow = 0

	media := widget.NewImageFrom(attachmentPreview(120, 72))
	media.Layout().Basis = 10
	media.Layout().Grow = 0

	actions := widget.Row(
		actionButton("Open"),
		actionButton("Save"),
		actionButton("Reply"),
	).SetGap(1)
	actions.Layout().Basis = 1
	actions.Layout().Grow = 0

	card := widget.Column(message, media, actions).SetGap(1)
	viewport := widget.NewViewport()
	viewport.SetChild(card)

	root := demo.Column(titled("Message With Markup And Media", viewport)).WithCancel(cancel)
	return root
}

func actionButton(label string) *widget.Button {
	button := widget.NewButton(label, nil)
	button.SetStyle(screen.Style{Fg: screen.RGB(235, 240, 245), Bg: screen.RGB(55, 65, 82)})
	button.Layout().Basis = len(label) + 2
	button.Layout().Grow = 0
	return button
}

func titled(title string, child tui.Widget) *widget.Border {
	box := widget.NewBorder(child)
	box.SetTitle(title)
	box.SetStyle(screen.Style{Fg: screen.RGB(120, 160, 220), Attrs: screen.Bold})
	return box
}

func attachmentPreview(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(25 + 160*x/maxInt(w-1, 1))
			g := uint8(35 + 175*y/maxInt(h-1, 1))
			b := uint8(90 + 120*(x+y)/maxInt(w+h-2, 1))
			if x > w/5 && x < 4*w/5 && y > h/4 && y < 3*h/4 {
				r, g, b = 230, 210, 120
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
