package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

type recordingOverlayCloser struct {
	*widget.Text
	closed bool
}

func (o *recordingOverlayCloser) Close() { o.closed = true }

type recordingDesktopNotifier struct {
	titles []string
	bodies []string
	err    error
}

func (n *recordingDesktopNotifier) Notify(title, body string) error {
	n.titles = append(n.titles, title)
	n.bodies = append(n.bodies, body)
	return n.err
}

func TestShellFallsBackToToastWhenDesktopNotificationFails(t *testing.T) {
	sh := &Shell{
		cfg:      config.Default(),
		mv:       &MainView{Root: widget.NewText("main")},
		notifier: &recordingDesktopNotifier{err: errors.New("unavailable")},
	}
	sh.Handle(input.FocusEvent{Focused: false})
	sh.NotifyIncomingMessage(store.Message{Author: "Mina", Content: "hello", ChannelID: 9})
	if got := len(sh.Toasts()); got != 1 {
		t.Fatalf("toast count after desktop failure = %d, want 1", got)
	}
	if sh.Toasts()[0].onActivate == nil {
		t.Fatal("fallback incoming-message toast must remain actionable")
	}
}

func TestShellRoutesIncomingMessageByWindowFocus(t *testing.T) {
	notifier := &recordingDesktopNotifier{}
	sh := &Shell{
		cfg:      config.Default(),
		mv:       &MainView{Root: widget.NewText("main")},
		notifier: notifier,
	}
	message := store.Message{Author: "Mina", Content: "hello", ChannelID: 9}

	sh.Handle(input.FocusEvent{Focused: false})
	sh.NotifyIncomingMessage(message)
	if len(notifier.titles) != 1 || len(sh.Toasts()) != 0 {
		t.Fatalf("unfocused notification = desktop %v, toasts %d; want desktop only", notifier.titles, len(sh.Toasts()))
	}

	sh.Handle(input.FocusEvent{Focused: true})
	sh.NotifyIncomingMessage(message)
	if len(notifier.titles) != 1 || len(sh.Toasts()) != 1 {
		t.Fatalf("focused notification = desktop %v, toasts %d; want in-app only", notifier.titles, len(sh.Toasts()))
	}
}

func TestIncomingNotificationClosesActionLayersBeforeNavigation(t *testing.T) {
	overlay := &recordingOverlayCloser{Text: widget.NewText("overlay")}
	_, cancel := context.WithCancel(context.Background())
	sh := &Shell{
		cfg:          config.Default(),
		mv:           &MainView{Root: widget.NewText("main")},
		overlay:      overlay,
		popup:        widget.NewText("popup"),
		viewerCancel: cancel,
	}
	sh.showIncomingMessageToast(store.Message{ChannelID: 9}, "Mina", "hello")
	sh.toasts[0].onActivate()
	if !overlay.closed || sh.overlay != nil || sh.popup != nil || sh.viewerCancel != nil {
		t.Fatalf("activation left action layers open: closed=%v overlay=%v popup=%v cancel=%v", overlay.closed, sh.overlay, sh.popup, sh.viewerCancel)
	}
}

func TestIncomingMessageToastOpensItsChannel(t *testing.T) {
	opened := store.ChannelID(0)
	toast := newExpiringToast("Mina", "hello", Styles{}, time.Now())
	toast.onActivate = func() { opened = 9 }
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}, toasts: []*Toast{toast}}

	if !sh.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("Enter did not handle the incoming-message toast")
	}
	if opened != 9 || len(sh.Toasts()) != 0 {
		t.Fatalf("opened=%d toasts=%d, want channel 9 and dismissed toast", opened, len(sh.Toasts()))
	}
}

func TestIncomingMessageToastClickOpensItsChannel(t *testing.T) {
	opened := store.ChannelID(0)
	toast := newExpiringToast("Mina", "hello", Styles{}, time.Now())
	toast.onActivate = func() { opened = 9 }
	toast.bounds.X, toast.bounds.Y, toast.bounds.W, toast.bounds.H = 4, 3, 20, 4
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}, toasts: []*Toast{toast}}

	if !sh.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: 5, Y: 4}) {
		t.Fatal("click did not handle the incoming-message toast")
	}
	if opened != 9 || len(sh.Toasts()) != 0 {
		t.Fatalf("opened=%d toasts=%d, want channel 9 and dismissed toast", opened, len(sh.Toasts()))
	}
}

func TestClickingOlderIncomingToastActivatesExactToast(t *testing.T) {
	opened := store.ChannelID(0)
	newest := newExpiringToast("new", "message", Styles{}, time.Now())
	newest.onActivate = func() { opened = 2 }
	newest.bounds.X, newest.bounds.Y, newest.bounds.W, newest.bounds.H = 0, 0, 20, 3
	older := newExpiringToast("old", "message", Styles{}, time.Now())
	older.onActivate = func() { opened = 1 }
	older.bounds.X, older.bounds.Y, older.bounds.W, older.bounds.H = 0, 4, 20, 3
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}, toasts: []*Toast{newest, older}}

	if !sh.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: 2, Y: 5}) {
		t.Fatal("click did not handle the older toast")
	}
	if opened != 1 || len(sh.Toasts()) != 1 || sh.Toasts()[0] != newest {
		t.Fatalf("opened=%d toasts=%v, want older activated and only newest retained", opened, sh.Toasts())
	}
}

func TestErrorToastDoesNotExpire(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}, now: func() time.Time { return now }}
	sh.ShowToast("send failed", errors.New("network unavailable"))
	now = now.Add(notificationTTL * 2)
	sh.Handle(input.TickEvent{})
	if len(sh.Toasts()) != 1 {
		t.Fatal("error toast expired before explicit dismissal")
	}
}

func TestShellCoalescesDuplicateErrorToasts(t *testing.T) {
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}}
	sh.ShowToast("Discord error", errors.New("JSON decoding failed: expected container"))
	sh.ShowToast("Other error", errors.New("network unavailable"))
	sh.ShowToast("Discord error", errors.New("JSON decoding failed: expected container"))

	if got := len(sh.Toasts()); got != 2 {
		t.Fatalf("toast count = %d, want two distinct errors", got)
	}
	toast := sh.Toasts()[0]
	if toast.title != "Discord error" || toast.repeats != 2 {
		t.Fatalf("coalesced toast = %#v, want newest Discord error repeated twice", toast)
	}
	buf := tui.New().Render(sh, tui.Size{W: 60, H: 12})
	if !bufferContains(buf, "Discord error ×2") {
		t.Fatal("coalesced toast did not render its repeat count")
	}
}

func TestShellStacksAndExpiresNotifications(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	sh := &Shell{
		cfg: config.Default(),
		mv:  &MainView{Root: widget.NewText("main")},
		now: func() time.Time { return now },
	}
	sh.ShowNotice("first", "one")
	now = now.Add(time.Second)
	sh.ShowNotice("second", "two")
	if got := len(sh.Toasts()); got != 2 {
		t.Fatalf("toast count = %d, want 2", got)
	}
	if sh.Toasts()[0].title != "second" {
		t.Fatalf("newest toast = %q, want second", sh.Toasts()[0].title)
	}

	now = now.Add(notificationTTL - time.Second)
	sh.Handle(input.TickEvent{})
	if got := len(sh.Toasts()); got != 1 {
		t.Fatalf("toast count before newest expiry = %d, want 1", got)
	}
	now = now.Add(time.Second)
	sh.Handle(input.TickEvent{})
	if got := len(sh.Toasts()); got != 0 {
		t.Fatalf("toast count after expiry = %d, want 0", got)
	}
}

func TestNotificationStackRendersInHoveringViewport(t *testing.T) {
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: widget.NewText("main")}}
	sh.ShowNotice("first notification", "one")
	sh.ShowNotice("second notification", "two")

	buf := tui.New().Render(sh, tui.Size{W: 60, H: 12})
	if !bufferContains(buf, "first notification") || !bufferContains(buf, "second notification") {
		t.Fatal("stacked notifications were not both rendered")
	}
}
