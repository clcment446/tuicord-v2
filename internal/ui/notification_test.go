package ui

import (
	"errors"
	"testing"
	"time"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

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
