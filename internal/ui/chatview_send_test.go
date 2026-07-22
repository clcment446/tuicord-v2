package ui

import (
	"fmt"
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
)

// TestChatViewShowsOptimisticEchoImmediately reproduces the send flow:
// App.Send appends a pending local echo, and the gateway later confirms it via
// ReplaceMessage. Each step must be visible on the very next draw.
func TestChatViewShowsOptimisticEchoImmediately(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "existing"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	draw := func() string {
		buf := screen.NewBuffer(60, 12)
		view.Draw(buf.Clip(buf.Bounds()))
		return bufferText(buf)
	}

	draw() // the view is warm, as it would be in a running session

	// App.Send: optimistic echo, ID still zero, identified only by nonce.
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "you", Content: "my new message", Nonce: "abc", Pending: true,
	})
	if got := draw(); !strings.Contains(got, "my new message") {
		t.Fatalf("draw after Send = %q, want the sent message to appear immediately", got)
	}

	// The gateway echoes it back and it reconciles onto the pending entry.
	confirmed := store.Message{
		ID: 2, ChannelID: 1, Author: "you", Content: "my new message", Nonce: "abc",
	}
	if !st.ReplaceMessage("abc", confirmed) {
		t.Fatal("ReplaceMessage found no pending echo to reconcile")
	}
	got := draw()
	if !strings.Contains(got, "my new message") {
		t.Errorf("draw after confirm = %q, want the message still present", got)
	}
	if strings.Contains(got, "sending") {
		t.Errorf("draw after confirm = %q, want the pending marker gone", got)
	}
}

// TestChatViewShowsFailedSend covers the other terminal state of a send.
func TestChatViewShowsFailedSend(t *testing.T) {
	st := store.New(0)
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	draw := func() string {
		buf := screen.NewBuffer(60, 12)
		view.Draw(buf.Clip(buf.Bounds()))
		return bufferText(buf)
	}

	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "you", Content: "doomed", Nonce: "n1", Pending: true,
	})
	draw()

	if !st.MarkFailed(1, "n1") {
		t.Fatal("MarkFailed found no pending message")
	}
	if got := draw(); !strings.Contains(got, "failed") {
		t.Errorf("draw after MarkFailed = %q, want the failure surfaced", got)
	}
}

// TestChatViewShowsGatewayMessageFromAnotherUser covers the plain append path:
// a message with no nonce that ReplaceMessage declines to reconcile.
func TestChatViewShowsGatewayMessageFromAnotherUser(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "first"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	draw := func() string {
		buf := screen.NewBuffer(60, 12)
		view.Draw(buf.Clip(buf.Bounds()))
		return bufferText(buf)
	}
	draw()

	st.AppendMessage(store.Message{ID: 2, ChannelID: 1, Author: "bob", Content: "incoming"})
	if got := draw(); !strings.Contains(got, "incoming") {
		t.Errorf("draw after a gateway message = %q, want it to appear immediately", got)
	}
}

func newFocusedScrollRegressionView(t *testing.T) (*store.Store, *ChatView, *screen.Buffer) {
	t.Helper()
	st := store.New(0)
	for id := store.MessageID(1); id <= 12; id++ {
		st.AppendMessage(store.Message{ID: id, ChannelID: 1, Author: "alice", Content: fmt.Sprintf("message-%02d", id)})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(40, 5)
	view.Draw(buf.Clip(buf.Bounds()))
	// Establish an explicit message focus, which used to make every later Draw
	// recompute the offset from the previous visibleStart.
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'K'}) {
		t.Fatal("K did not establish explicit focus")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	return st, view, buf
}

func TestChatViewDrawPreservesVimScrollAndJumpOffsets(t *testing.T) {
	_, view, buf := newFocusedScrollRegressionView(t)
	draw := func() { view.Draw(buf.Clip(buf.Bounds())) }

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'}) {
		t.Fatal("k was not handled")
	}
	want := view.bottomScroll.Offset()
	if want == 0 {
		t.Fatal("k did not move away from the live edge")
	}
	draw()
	if got := view.bottomScroll.Offset(); got != want {
		t.Fatalf("offset after k and Draw = %d, want %d", got, want)
	}
	// An unchanged subsequent draw must also leave the user-selected offset
	// alone rather than repeatedly applying an explicit-focus anchor.
	draw()
	if got := view.bottomScroll.Offset(); got != want {
		t.Fatalf("offset after unchanged Draw = %d, want %d", got, want)
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) {
		t.Fatal("j was not handled")
	}
	want = view.bottomScroll.Offset()
	draw()
	if got := view.bottomScroll.Offset(); got != want {
		t.Fatalf("offset after j and Draw = %d, want %d", got, want)
	}

	view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'})
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'G', Mods: input.Shift}) {
		t.Fatal("G was not handled")
	}
	draw()
	if got := view.bottomScroll.Offset(); got != 0 {
		t.Fatalf("offset after G and Draw = %d, want live edge 0", got)
	}
}

func TestChatViewFocusedAppendFollowsLiveEdgeOrPreservesReadingTop(t *testing.T) {
	t.Run("live edge shows incoming and optimistic appends", func(t *testing.T) {
		st, view, buf := newFocusedScrollRegressionView(t)
		view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'G', Mods: input.Shift})
		view.Draw(buf.Clip(buf.Bounds()))

		st.AppendMessage(store.Message{ID: 13, ChannelID: 1, Author: "bob", Content: "incoming-live"})
		view.Draw(buf.Clip(buf.Bounds()))
		if got := view.bottomScroll.Offset(); got != 0 {
			t.Fatalf("offset after incoming append = %d, want live edge 0", got)
		}
		if got := bufferText(buf); !strings.Contains(got, "incoming-live") {
			t.Fatalf("incoming append missing at live edge: %q", got)
		}

		st.AppendMessage(store.Message{ChannelID: 1, Author: "you", Content: "optimistic-live", Nonce: "pending", Pending: true})
		view.Draw(buf.Clip(buf.Bounds()))
		if got := view.bottomScroll.Offset(); got != 0 {
			t.Fatalf("offset after optimistic append = %d, want live edge 0", got)
		}
		if got := bufferText(buf); !strings.Contains(got, "optimistic-live") {
			t.Fatalf("optimistic append missing at live edge: %q", got)
		}
	})

	t.Run("append while reading preserves visible top", func(t *testing.T) {
		st, view, buf := newFocusedScrollRegressionView(t)
		view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'})
		view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'})
		view.Draw(buf.Clip(buf.Bounds()))
		if view.bottomScroll.Offset() == 0 {
			t.Fatal("test did not enter older content")
		}
		start := view.visibleStart
		before := bufferText(buf)

		st.AppendMessage(store.Message{ID: 13, ChannelID: 1, Author: "bob", Content: "incoming-below"})
		view.Draw(buf.Clip(buf.Bounds()))
		if got := view.visibleStart; got != start {
			t.Fatalf("visible start after append = %d, want preserved top %d", got, start)
		}
		if got := bufferText(buf); got != before {
			t.Fatalf("viewport changed while reading after append:\nbefore: %q\nafter:  %q", before, got)
		}
	})
}
