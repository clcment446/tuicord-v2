package main

import (
	"context"
	"fmt"
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
	if root == nil {
		os.Exit(1)
	}

	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 72, H: 18})))
		return
	}

	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	left := widget.NewText("Drag the vertical divider with the mouse.\nAlt+Left/Right also resizes it.\nPress Esc while dragging to cancel.")
	rightBottom := widget.NewText("The modal title bar is draggable too.\nq quits the example.")

	leftBox := widget.NewBorder(left)
	leftBox.SetTitle("Split")

	rightBox := widget.NewBorder(rightBottom)
	rightBox.SetTitle("Pane")

	imgPath := "/home/clement/Pictures/avatar.jpg"

	imageWidget := widget.NewKittyImage(imgPath)
	imageWidget.Layout().Grow = 1

	split := widget.NewSplit(leftBox, rightBox).Basis(30).MinFirst(18).MaxFirst(48)
	split.SetStyle(screen.Style{Fg: screen.RGB(120, 180, 255), Attrs: screen.Bold})

	modal := widget.NewModal("Avatar", imageWidget)

	// Larger modal = more terminal cells = higher displayed Kitty resolution.
	// Increase these if your terminal window is large enough.
	modal.SetSize(56, 24)

	modal.SetStyle(screen.Style{
		Fg:    screen.RGB(255, 210, 120),
		Attrs: screen.Bold,
	})

	root := demo.Column(split, modal).WithCancel(cancel)

	split.Layout().Grow = 1
	modal.Layout().Grow = 1

	return root
}
