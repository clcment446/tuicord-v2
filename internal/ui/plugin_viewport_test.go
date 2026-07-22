package ui

import (
	"testing"

	"awesomeProject/internal/plugin"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
)

func TestPluginViewportPreservesVimInsertFocus(t *testing.T) {
	runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
	enterVimInput(t, runtime, shell)
	shell.OpenPluginViewport("AutoBot", []string{"running"}, []plugin.ViewportAction{{ID: "pause", Label: "Pause"}}, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	if shell.editor.phase != editorInput || runtime.Focus.Focused() != mv.composer || !mv.composer.CanFocus() {
		t.Fatalf("viewport changed Vim input ownership: phase=%v focus=%T canFocus=%v", shell.editor.phase, runtime.Focus.Focused(), mv.composer.CanFocus())
	}
}

func TestPluginViewportDoesNotBlockVimInsertEntry(t *testing.T) {
	runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
	shell.OpenPluginViewport("AutoBot", []string{"running"}, nil, nil)
	// Model a transient focus rebuild while the plugin refreshes its panel.
	runtime.Focus.Clear()

	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i was not handled while plugin viewport was visible")
	}
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	if shell.editor.phase != editorInput || runtime.Focus.Focused() != mv.composer {
		t.Fatalf("insert entry with viewport = phase %v focus %T, want input/composer", shell.editor.phase, runtime.Focus.Focused())
	}
}

func TestPluginViewportDragKeepsMovedPositionAfterRender(t *testing.T) {
	runtime, shell, _, _ := newVimFocusHarness(t, vimTestConfig())
	shell.OpenPluginViewport("AutoBot", []string{"running"}, nil, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	viewport, ok := shell.popup.(*pluginViewport)
	if !ok {
		t.Fatalf("popup = %T, want plugin viewport", shell.popup)
	}
	start := viewport.last
	if !runtime.Handle(input.MouseEvent{X: start.X + 2, Y: start.Y, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("title-bar press was not handled")
	}
	if !runtime.Handle(input.MouseEvent{X: start.X + 12, Y: start.Y + 4, Btn: input.ButtonLeft, Kind: input.MouseMotion}) {
		t.Fatal("drag motion was not handled")
	}
	if !runtime.Handle(input.MouseEvent{X: start.X + 12, Y: start.Y + 4, Btn: input.ButtonLeft, Kind: input.MouseRelease}) {
		t.Fatal("drag release was not handled")
	}
	runtime.Render(shell, tui.Size{W: 80, H: 24})

	if got, want := viewport.last.X, start.X+10; got != want {
		t.Fatalf("viewport x after drag/render = %d, want %d", got, want)
	}
	if got, want := viewport.last.Y, start.Y+4; got != want {
		t.Fatalf("viewport y after drag/render = %d, want %d", got, want)
	}

	shell.OpenPluginViewport("AutoBot", []string{"updated"}, nil, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	viewport, ok = shell.popup.(*pluginViewport)
	if !ok {
		t.Fatalf("updated popup = %T, want plugin viewport", shell.popup)
	}
	if got, want := viewport.last.X, start.X+10; got != want {
		t.Fatalf("viewport x after refresh = %d, want preserved %d", got, want)
	}
	if got, want := viewport.last.Y, start.Y+4; got != want {
		t.Fatalf("viewport y after refresh = %d, want preserved %d", got, want)
	}
}

func TestPluginViewportCollapseShrinksBoundsAndStopsClaimingOldArea(t *testing.T) {
	runtime, shell, _, _ := newVimFocusHarness(t, vimTestConfig())
	shell.OpenPluginViewport("AutoBot", []string{"running", "still running"}, nil, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	viewport := shell.popup.(*pluginViewport)
	start := viewport.last

	if !runtime.Handle(input.MouseEvent{X: start.X + start.W - 2, Y: start.Y, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("collapse press was not handled")
	}
	runtime.Render(shell, tui.Size{W: 80, H: 24})

	if got, want := viewport.last.H, 1; got != want {
		t.Fatalf("collapsed viewport height = %d, want %d", got, want)
	}
	if got, want := viewport.last.W, start.W; got != want {
		t.Fatalf("collapsed viewport width = %d, want preserved %d", got, want)
	}
	if shell.OverlayAt(start.X+1, start.Y+start.H-1) != nil {
		t.Fatal("collapsed viewport still claimed its old content area")
	}

	if !runtime.Handle(input.MouseEvent{X: viewport.last.X + viewport.last.W - 2, Y: viewport.last.Y, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("expand press was not handled")
	}
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	if got, want := viewport.last.H, start.H; got != want {
		t.Fatalf("expanded viewport height = %d, want restored %d", got, want)
	}
}

func TestPluginViewportRefreshWhileCollapsedKeepsRestoreSizeAndLatestContent(t *testing.T) {
	runtime, shell, _, _ := newVimFocusHarness(t, vimTestConfig())
	shell.OpenPluginViewport("AutoBot", []string{"old status"}, nil, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	viewport := shell.popup.(*pluginViewport)
	expandedHeight := viewport.last.H
	start := viewport.last

	if !runtime.Handle(input.MouseEvent{X: start.X + start.W - 2, Y: start.Y, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("collapse press was not handled")
	}
	runtime.Render(shell, tui.Size{W: 80, H: 24})

	shell.OpenPluginViewport("AutoBot", []string{"new status"}, nil, nil)
	runtime.Render(shell, tui.Size{W: 80, H: 24})
	viewport = shell.popup.(*pluginViewport)
	if !viewport.modal.Collapsed() || viewport.last.H != 1 {
		t.Fatalf("collapsed refresh geometry = %+v, collapsed=%v", viewport.last, viewport.modal.Collapsed())
	}

	if !runtime.Handle(input.MouseEvent{X: viewport.last.X + viewport.last.W - 2, Y: viewport.last.Y, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("expand press after refresh was not handled")
	}
	buf := runtime.Render(shell, tui.Size{W: 80, H: 24})
	if viewport.last.H != expandedHeight {
		t.Fatalf("expanded height after collapsed refresh = %d, want %d", viewport.last.H, expandedHeight)
	}
	found := false
	for y := viewport.last.Y; y < viewport.last.Y+viewport.last.H; y++ {
		for x := viewport.last.X; x < viewport.last.X+viewport.last.W; x++ {
			if buf.Cell(x, y).Content == "n" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expanded refreshed viewport did not render latest content")
	}
}
