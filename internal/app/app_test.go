package app

import (
	"errors"
	"sync"
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
)

// syncPoster runs posted closures immediately, as if already on the UI goroutine.
type syncPoster struct{}

func (syncPoster) Post(fn func()) { fn() }

// fakeSender records sends and returns a preset error.
type fakeSender struct {
	mu   sync.Mutex
	sent int
	err  error
	done chan struct{}
}

func (f *fakeSender) SendMessageComplex(discord.ChannelID, api.SendMessageData) (*discord.Message, error) {
	f.mu.Lock()
	f.sent++
	f.mu.Unlock()
	if f.done != nil {
		close(f.done)
	}
	return &discord.Message{}, f.err
}

func newTestApp(send sender) *App {
	return &App{
		store: store.New(0),
		ui:    syncPoster{},
		send:  send,
	}
}

func TestSendAppendsOptimisticPending(t *testing.T) {
	fs := &fakeSender{done: make(chan struct{})}
	a := newTestApp(fs)
	a.SetActive(1, 42)

	a.Send("hello")

	msgs := a.store.Messages(42)
	if len(msgs) != 1 {
		t.Fatalf("want 1 optimistic message, got %d", len(msgs))
	}
	if !msgs[0].Pending || msgs[0].Content != "hello" {
		t.Errorf("optimistic message = %+v, want pending 'hello'", msgs[0])
	}
	<-fs.done // deliver goroutine ran
}

func TestDeliverMarksFailedOnError(t *testing.T) {
	fs := &fakeSender{err: errors.New("boom")}
	a := newTestApp(fs)
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	// Call deliver directly (synchronous) to avoid racing with Send's goroutine;
	// syncPoster then applies MarkFailed on this goroutine.
	a.deliver(42, "hi", "n1")

	msgs := a.store.Messages(42)
	if len(msgs) != 1 || !msgs[0].Failed || msgs[0].Pending {
		t.Errorf("message after failed deliver = %+v, want failed and not pending", msgs[0])
	}
}

func TestSendIgnoredWithoutActiveChannel(t *testing.T) {
	fs := &fakeSender{}
	a := newTestApp(fs)

	a.Send("hello") // no active channel

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.sent != 0 {
		t.Errorf("sent %d messages without an active channel, want 0", fs.sent)
	}
}

func TestReconcileReplacesPendingOnGatewayEcho(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.SetActive(1, 42)
	a.store.AppendMessage(store.Message{ChannelID: 42, Nonce: "n1", Pending: true, Content: "hi"})

	echo := store.Message{ID: 99, ChannelID: 42, Nonce: "n1", Content: "hi"}
	if !a.store.ReplaceMessage(echo.Nonce, echo) {
		a.store.AppendMessage(echo)
	}

	msgs := a.store.Messages(42)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message after reconcile, got %d", len(msgs))
	}
	if msgs[0].Pending || msgs[0].ID != 99 {
		t.Errorf("reconciled message = %+v, want id 99 not pending", msgs[0])
	}
}
