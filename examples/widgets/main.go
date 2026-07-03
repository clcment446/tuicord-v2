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
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 64, H: 16})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc) tui.Widget {
	guilds := widget.NewList([]string{
		"# general",
		"# tui-dev",
		"# unicode-🎉",
		"# releases",
		"# support",
	})
	guilds.SetSelectedStyle(screen.Style{Attrs: screen.Reverse | screen.Bold})

	chat := widget.NewViewport()
	chat.SetLines([]string{
		"mira: resize the terminal and the panes stay in cells.",
		"jules: paste text into the input; it arrives atomically.",
		"ren: wide glyphs stay aligned: テスト 🎉 👩‍💻",
		"sam: Tab moves focus, arrows scroll/select, q quits.",
	})

	composer := widget.NewTextInput("type a message...")
	composer.SetFocused(false)
	composer.SetStyle(screen.Style{Fg: screen.RGB(220, 220, 220)})

	left := widget.NewBorder(guilds)
	left.SetTitle("Channels")
	main := widget.NewBorder(chat)
	main.SetTitle("Messages")
	top := widget.NewSplit(left, main).Basis(18).MinFirst(12).MaxFirst(30)

	inputBox := widget.NewBorder(composer)
	inputBox.SetTitle("Composer")
	root := demo.Column(top, inputBox).WithCancel(cancel)
	top.Layout().Grow = 1
	inputBox.Layout().Basis = 3
	inputBox.Layout().Grow = 0
	return root
}
