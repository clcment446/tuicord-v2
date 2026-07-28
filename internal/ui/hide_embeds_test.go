package ui

import (
	"testing"

	"awesomeProject/internal/store"
)

func TestToggleFocusedRichContentHidesAllRichBlocks(t *testing.T) {
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, Kind: store.ChannelText})
	msg := store.Message{
		ID: 9, ChannelID: 1, Content: "keep this",
		Attachments:   []store.Attachment{{Filename: "photo.png", URL: "https://cdn.example/photo.png"}},
		Embeds:        []store.Embed{{Title: "embed"}},
		ComponentTree: []store.ComponentNode{{Kind: store.ComponentContainer, Children: []store.ComponentNode{{Kind: store.ComponentTextDisplay, Content: "v2"}}}},
	}
	st.AppendMessage(msg)
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.focusedMessage = msg
	view.focusedMessageSet = true

	if !view.ToggleFocusedRichContent() || !view.hiddenRichContent[messagePlacementPrefix(msg)] {
		t.Fatal("toggle did not hide focused message rich content")
	}
	body, _ := view.renderBody(msg, 1, 80)
	if len(body) != 1 || len(body[0].segments) == 0 || body[0].media != nil || len(body[0].actions) != 0 {
		t.Fatalf("hidden body = %+v, want text-only body", body)
	}
	if !view.ToggleFocusedRichContent() || view.hiddenRichContent[messagePlacementPrefix(msg)] {
		t.Fatal("second toggle did not restore rich content")
	}
}
