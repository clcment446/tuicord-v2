package app

import (
	"errors"
	"testing"

	"awesomeProject/internal/store"
)

// fakeInteractionPoster records the interaction payload and signals done. A
// non-nil release channel blocks completion until the test closes it.
type fakeInteractionPoster struct {
	payload componentInteraction
	err     error
	done    chan struct{}
	release chan struct{}
}

func (f *fakeInteractionPoster) postComponentInteraction(p componentInteraction) error {
	f.payload = p
	close(f.done)
	if f.release != nil {
		<-f.release
	}
	return f.err
}

func componentTestMessage() store.Message {
	return store.Message{
		ID:            900,
		ChannelID:     42,
		AuthorID:      77,
		ApplicationID: 555,
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{{
				Kind:     store.ComponentSelect,
				RawType:  3,
				CustomID: "sell_items",
			}},
		}},
	}
}

func TestSubmitComponentPostsInteractionPayload(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi, sessionID: "sess-1"}
	a.SetActive(7, 42)
	msg := componentTestMessage()
	a.store.AppendMessage(msg)

	// Act
	a.SubmitComponent(ComponentSubmit{
		Message:       msg,
		ComponentType: 3,
		CustomID:      "sell_items",
		Values:        []string{"101", "102"},
	})
	<-fi.done

	// Assert
	p := fi.payload
	if p.Type != 3 || p.ChannelID != "42" || p.MessageID != "900" {
		t.Fatalf("payload = %+v, want type 3 for message 900 in channel 42", p)
	}
	if p.ApplicationID != "555" {
		t.Fatalf("application id = %q, want 555 (message ApplicationID)", p.ApplicationID)
	}
	if p.GuildID != "7" || p.SessionID != "sess-1" || p.Nonce == "" {
		t.Fatalf("payload routing = %+v, want guild 7, session sess-1, nonce set", p)
	}
	if p.Data.ComponentType != 3 || p.Data.CustomID != "sell_items" {
		t.Fatalf("data = %+v, want string select sell_items", p.Data)
	}
	if got := len(p.Data.Values); got != 2 || p.Data.Values[0] != "101" {
		t.Fatalf("values = %v, want [101 102]", p.Data.Values)
	}
}

func TestSubmitComponentFallsBackToAuthorAndButton(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}
	a.SetActive(DirectMessagesGuildID, 42)
	msg := componentTestMessage()
	msg.ApplicationID = 0

	// Act
	a.SubmitComponent(ComponentSubmit{Message: msg, CustomID: "confirm"})
	<-fi.done

	// Assert
	p := fi.payload
	if p.ApplicationID != "77" {
		t.Fatalf("application id = %q, want author fallback 77", p.ApplicationID)
	}
	if p.GuildID != "" {
		t.Fatalf("guild id = %q, want empty for DMs", p.GuildID)
	}
	if p.Data.ComponentType != 2 {
		t.Fatalf("component type = %d, want button fallback 2", p.Data.ComponentType)
	}
}

func TestSubmitComponentMarksStateAndReportsErrors(t *testing.T) {
	// Arrange
	fi := &fakeInteractionPoster{done: make(chan struct{}), release: make(chan struct{}), err: errors.New("boom")}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}
	errCh := make(chan error, 1)
	a.OnError(func(err error) { errCh <- err })
	msg := componentTestMessage()
	a.store.AppendMessage(msg)

	componentState := func() store.ComponentState {
		msgs := a.store.Messages(42)
		return msgs[len(msgs)-1].ComponentTree[0].Children[0].State
	}

	// Act
	a.SubmitComponent(ComponentSubmit{Message: msg, ComponentType: 3, CustomID: "sell_items"})
	<-fi.done // request captured; completion is blocked on release
	if got := componentState(); got != store.ComponentStatePending {
		t.Fatalf("component state = %v, want pending right after submit", got)
	}
	close(fi.release)
	reported := <-errCh // completion ran; the state write precedes OnError

	// Assert
	if got := componentState(); got != store.ComponentStateError {
		t.Fatalf("component state = %v, want error", got)
	}
	if reported == nil || reported.Error() != "boom" {
		t.Fatalf("reported error = %v, want boom", reported)
	}
}

func TestSubmitComponentIgnoresIncompleteActions(t *testing.T) {
	fi := &fakeInteractionPoster{done: make(chan struct{})}
	a := &App{store: store.New(0), ui: syncPoster{}, interact: fi}

	a.SubmitComponent(ComponentSubmit{Message: store.Message{ID: 1, ChannelID: 2}})         // no custom id
	a.SubmitComponent(ComponentSubmit{Message: store.Message{ChannelID: 2}, CustomID: "x"}) // pending message
	select {
	case <-fi.done:
		t.Fatal("incomplete action should not post an interaction")
	default:
	}
}
