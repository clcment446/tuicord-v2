package main

import (
	"context"
	"fmt"
	"os"

	"awesomeProject/examples/internal/demo"
	"awesomeProject/examples/internal/imagedemo"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := build(cancel)
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 84, H: 22})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	low, high := sourcePair()
	low.SetPixelSize(68, 28)
	high.SetPixelSize(612, 504)

	left := titled("low payload: 68x28 px", low)
	right := titled("high payload: 612x504 px", high)
	panels := widget.Row(left, right).SetGap(2)
	panels.Layout().Grow = 1

	notes := widget.NewText("Kitty quality comparison. Both panes are regular image widgets; only their transmitted pixel payload differs.")
	notes.SetStyle(screen.Style{Fg: screen.RGB(210, 215, 225)})
	notes.Layout().Basis = 2
	notes.Layout().Grow = 0

	root := demo.Column(notes, panels).WithCancel(cancel)
	panels.Layout().Grow = 1
	return root
}

func sourcePair() (*widget.Image, *widget.Image) {
	if path := imagedemo.LocalImagePath(); path != "" {
		return widget.NewKittyImage(path), widget.NewKittyImage(path)
	}
	img := imagedemo.Radial(960, 640)
	return widget.NewKittyImageFrom(img), widget.NewKittyImageFrom(img)
}

func titled(title string, child tui.Widget) *widget.Border {
	box := widget.NewBorder(child)
	box.SetTitle(title)
	box.SetStyle(screen.Style{Fg: screen.RGB(120, 180, 255), Attrs: screen.Bold})
	return box
}
