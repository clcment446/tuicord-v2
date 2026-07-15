package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

// TestSendThenGatewayEchoReconciles reproduces the full send path: Send appends
// a pending local echo, then Discord echoes the message back over the gateway
// and it must reconcile onto that echo rather than appearing late or twice.
func TestSendThenGatewayEchoReconciles(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.activeChannel = 5
	a.SetActive(0, 5)

	a.Send("hello")

	msgs := a.store.Messages(5)
	if len(msgs) != 1 {
		t.Fatalf("after Send the store holds %d messages, want the optimistic echo", len(msgs))
	}
	if !msgs[0].Pending {
		t.Error("the local echo is not marked pending")
	}
	nonce := msgs[0].Nonce
	if nonce == "" {
		t.Fatal("the local echo has no nonce, so the gateway echo cannot reconcile onto it")
	}

	// Discord echoes the message back carrying the same nonce.
	a.handleMessageCreate(&gateway.MessageCreateEvent{
		Message: discord.Message{
			ID:        discord.MessageID(99),
			ChannelID: discord.ChannelID(5),
			Content:   "hello",
			Nonce:     nonce,
			Author:    discord.User{ID: discord.UserID(1), Username: "you"},
		},
	})

	msgs = a.store.Messages(5)
	if len(msgs) != 1 {
		t.Fatalf("after the gateway echo the store holds %d messages, want 1 reconciled "+
			"message; the echo did not match the pending entry", len(msgs))
	}
	if msgs[0].Pending {
		t.Error("the message is still pending after the gateway confirmed it")
	}
	if msgs[0].ID != 99 {
		t.Errorf("reconciled message ID = %d, want 99", msgs[0].ID)
	}
}

// TestSendThenHistoryLoadKeepsPendingEcho pins the ordering hazard that made a
// sent message vanish.
//
// Channel history loads asynchronously and can take seconds. A send during that
// window appends a pending echo, and the history page — fetched before the send
// and so not containing it — then replaced the whole ring, silently dropping
// the message. It reappeared only once the user sent something else.
func TestSendThenHistoryLoadKeepsPendingEcho(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.activeChannel = 5
	a.SetActive(0, 5)

	a.Send("hello")
	if got := len(a.store.Messages(5)); got != 1 {
		t.Fatalf("after Send the store holds %d messages, want the optimistic echo", got)
	}

	// A history page fetched before the send lands afterwards.
	a.store.SetMessages(5, []store.Message{{ID: 1, Content: "older"}})

	msgs := a.store.Messages(5)
	var echo *store.Message
	for i := range msgs {
		if msgs[i].Pending {
			echo = &msgs[i]
		}
	}
	if echo == nil {
		t.Fatalf("the history load dropped the pending local echo: store holds %d "+
			"messages, none pending. The user's sent message disappears.", len(msgs))
	}
	if echo.Content != "hello" {
		t.Errorf("surviving echo content = %q, want %q", echo.Content, "hello")
	}
	// It must remain the newest entry so it renders at the bottom.
	if !msgs[len(msgs)-1].Pending {
		t.Error("the pending echo is not the newest message, so it would not render " +
			"below the loaded history")
	}
}

// TestHistoryLoadThenGatewayEchoStillReconciles pins that an echo carried
// across a history load can still be confirmed by nonce afterwards — otherwise
// it would stay stuck showing "sending" forever.
func TestHistoryLoadThenGatewayEchoStillReconciles(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.activeChannel = 5
	a.SetActive(0, 5)

	a.Send("hello")
	nonce := a.store.Messages(5)[0].Nonce

	a.store.SetMessages(5, []store.Message{{ID: 1, Content: "older"}})

	a.handleMessageCreate(&gateway.MessageCreateEvent{
		Message: discord.Message{
			ID: discord.MessageID(99), ChannelID: discord.ChannelID(5),
			Content: "hello", Nonce: nonce,
			Author: discord.User{ID: discord.UserID(1), Username: "you"},
		},
	})

	msgs := a.store.Messages(5)
	if len(msgs) != 2 {
		t.Fatalf("store holds %d messages, want the history entry plus one reconciled "+
			"message (a duplicate means the echo was not matched)", len(msgs))
	}
	for _, m := range msgs {
		if m.Pending {
			t.Error("the echo is still pending after the gateway confirmed it")
		}
	}
}

// TestHistoryLoadDropsConfirmedEcho pins the other direction: when the history
// page already contains the sent message, the local echo must not survive as a
// duplicate alongside it.
func TestHistoryLoadDropsConfirmedEcho(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.activeChannel = 5
	a.SetActive(0, 5)

	a.Send("hello")
	nonce := a.store.Messages(5)[0].Nonce

	// A history page fetched after the send: it already contains the message.
	a.store.SetMessages(5, []store.Message{
		{ID: 1, Content: "older"},
		{ID: 99, Content: "hello", Nonce: nonce},
	})

	msgs := a.store.Messages(5)
	if len(msgs) != 2 {
		t.Fatalf("store holds %d messages, want 2: the echo is already in the history "+
			"and must not be carried over as a duplicate", len(msgs))
	}
	for _, m := range msgs {
		if m.Pending {
			t.Error("a confirmed message is still marked pending")
		}
	}
}

// TestHistoryLoadKeepsFailedEcho pins that a failed send survives a history
// load too: dropping it would silently hide the failure from the user.
func TestHistoryLoadKeepsFailedEcho(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.activeChannel = 5
	a.SetActive(0, 5)

	a.Send("doomed")
	nonce := a.store.Messages(5)[0].Nonce
	if !a.store.MarkFailed(5, nonce) {
		t.Fatal("MarkFailed found no pending message")
	}

	a.store.SetMessages(5, []store.Message{{ID: 1, Content: "older"}})

	for _, m := range a.store.Messages(5) {
		if m.Failed {
			return
		}
	}
	t.Error("the history load dropped the failed send, hiding the failure")
}
