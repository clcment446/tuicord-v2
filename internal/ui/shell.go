package ui

import (
	"context"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// Shell is the root widget. It shows the main view and can swap in a
// full-screen overlay (quick switcher or help). Overlays are implemented as a
// tree swap rather than a z-ordered layer, which the toolkit supports directly:
// Children returns whichever subtree is active, so focus, hit-testing, and
// drawing all follow.
type Shell struct {
	mv      *MainView
	app     *app.App
	cfg     config.Config
	styles  Styles
	overlay tui.Widget // nil = show the main view
	toast   *Toast
	cancel  context.CancelFunc
	node    layout.Node
}

// NewShell wraps a MainView with overlay handling.
func NewShell(a *app.App, mv *MainView, cfg config.Config, styles Styles, cancel context.CancelFunc) *Shell {
	return &Shell{mv: mv, app: a, cfg: cfg, styles: styles, cancel: cancel, node: layout.Node{Grow: 1}}
}

func (s *Shell) current() tui.Widget {
	if s.overlay != nil {
		return s.overlay
	}
	return s.mv.Root
}

// Children exposes the active subtree.
func (s *Shell) Children() []tui.Widget { return []tui.Widget{s.current()} }

// Measure delegates to the active subtree.
func (s *Shell) Measure(avail tui.Size) tui.Size { return s.current().Measure(avail) }

// Layout returns the shell node wrapping the active subtree.
func (s *Shell) Layout() *layout.Node {
	s.node.Children = []*layout.Node{s.current().Layout()}
	return &s.node
}

// Draw is a no-op; children draw themselves.
func (s *Shell) Draw(screen.Region) {}

func (s *Shell) DrawOverlay(r screen.Region) {
	if s != nil && s.toast != nil {
		s.toast.Draw(r)
	}
}

// Handle routes global shortcuts and overlay dismissal, delegating everything
// else to the active subtree.
func (s *Shell) Handle(ev tui.Event) bool {
	key, isKey := ev.(input.KeyEvent)

	if s.toast != nil && s.toast.Handle(ev) {
		if s.toast.wantsDismiss(ev) {
			s.toast = nil
		}
		return true
	}

	if s.overlay != nil {
		// The help overlay has no focusable widgets, so its keys arrive here;
		// the quick switcher's dismissal (Esc) arrives here via root fallback.
		if isKey && (keyMatches(key, s.cfg.Keys.Help) || key.Key == input.KeyEsc) {
			s.closeOverlay()
			return true
		}
		return s.overlay.Handle(ev)
	}

	if isKey {
		switch {
		case keyMatches(key, s.cfg.Keys.QuickSwitcher):
			s.openQuickSwitcher()
			return true
		case keyMatches(key, s.cfg.Keys.Help):
			s.overlay = NewHelpOverlay(s.cfg)
			return true
		}
	}
	return s.mv.Root.Handle(ev)
}

func (s *Shell) openQuickSwitcher() {
	s.overlay = NewQuickSwitcher(s.app.Store(), s.styles,
		func(guild store.GuildID, channel store.ChannelID) {
			s.app.SetActive(guild, channel)
			s.mv.RefreshChannels()
		},
		s.closeOverlay,
	)
}

func (s *Shell) closeOverlay() { s.overlay = nil }

// ShowToast displays a dismissible error popup over the active view.
func (s *Shell) ShowToast(title string, err error) {
	if s == nil || err == nil {
		return
	}
	s.toast = NewToast(title, err.Error(), s.styles)
}

// Toast returns the current popup, if any.
func (s *Shell) Toast() *Toast {
	if s == nil {
		return nil
	}
	return s.toast
}
