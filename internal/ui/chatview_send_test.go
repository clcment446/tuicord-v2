package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
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
