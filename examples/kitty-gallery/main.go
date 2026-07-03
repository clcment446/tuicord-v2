package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"awesomeProject/examples/internal/demo"
	"awesomeProject/examples/internal/imagedemo"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

type item struct {
	title string
	img   *widget.Image
}

type config struct {
	cols int
	rows int
	gap  int
}

func main() {
	var cfg config
	flag.IntVar(&cfg.cols, "cols", envInt("TUI_GALLERY_COLS", 2), "gallery columns")
	flag.IntVar(&cfg.rows, "rows", envInt("TUI_GALLERY_ROWS", 0), "gallery rows; 0 derives from image count")
	flag.IntVar(&cfg.gap, "gap", envInt("TUI_GALLERY_GAP", 1), "gap between panels in cells")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := build(cancel, cfg)
	if os.Getenv("TUI_EXAMPLE_RENDER") == "1" {
		fmt.Println(demo.Dump(tui.New().Render(root, tui.Size{W: 96, H: 34})))
		return
	}
	if err := tui.New().RunContext(ctx, root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(cancel context.CancelFunc, cfg config) tui.Widget {
	items := galleryItems()
	gallery := buildGallery(items, cfg)
	gallery.Layout().Grow = 1

	header := widget.NewText("Kitty responsive image gallery. Resize the terminal; the images remain regular widgets in the layout.")
	header.SetStyle(screen.Style{Fg: screen.RGB(210, 215, 225)})
	header.Layout().Basis = 2
	header.Layout().Grow = 0

	root := demo.Column(header, gallery).WithCancel(cancel)
	gallery.Layout().Grow = 1
	return root
}

func galleryItems() []item {
	items := []item{
		{title: "generated gradient", img: widget.NewKittyImageFrom(imagedemo.Gradient(960, 640))},
		{title: "generated radial", img: widget.NewKittyImageFrom(imagedemo.Radial(960, 640))},
		{title: "generated protocol", img: widget.NewKittyImageFrom(imagedemo.Protocol(960, 640))},
	}
	if path := imagedemo.LocalImagePath(); path != "" {
		items = append(items, item{title: path, img: widget.NewKittyImage(path)})
	} else {
		items = append(items, item{title: "generated checker", img: widget.NewKittyImageFrom(imagedemo.Checker(960, 640))})
	}
	return items
}

func buildGallery(items []item, cfg config) *widget.Node {
	cols := maxInt(cfg.cols, 1)
	rows := cfg.rows
	if rows <= 0 {
		rows = (len(items) + cols - 1) / cols
	}
	rows = maxInt(rows, 1)
	limit := minInt(len(items), cols*rows)

	grid := widget.Column()
	grid.SetGap(maxInt(cfg.gap, 0))
	var rowWidgets []tui.Widget
	for start := 0; start < limit; start += cols {
		end := minInt(start+cols, limit)
		var panels []tui.Widget
		for _, it := range items[start:end] {
			panels = append(panels, titled(it.title, it.img))
		}
		row := widget.Row(panels...)
		row.SetGap(maxInt(cfg.gap, 0))
		row.Layout().Grow = 1
		rowWidgets = append(rowWidgets, row)
	}
	grid.SetChildren(rowWidgets...)
	return grid
}

func titled(title string, child tui.Widget) *widget.Border {
	box := widget.NewBorder(child)
	box.SetTitle(title)
	box.SetStyle(screen.Style{Fg: screen.RGB(120, 180, 255), Attrs: screen.Bold})
	return box
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
