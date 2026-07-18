package ui

import (
	"testing"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
)

// A loading image must reserve the same number of rows the loaded image will
// occupy, so the async load swaps in place instead of growing the message and
// shifting the reader's viewport.
func TestLoadingPlaceholderReservesImageHeight(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/big.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "big.png", ContentType: "image/png", Size: 2048, W: 800, H: 600}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	base := view.styles.Cell("messages.content")
	msg := st.Messages(1)[0]

	view.media = map[string]*chatMediaState{url: {loading: true}}
	loadingRows := len(view.renderMedia(msg, 40, base))

	view.media = map[string]*chatMediaState{url: {img: solidTestImage(80, 60)}}
	loadedRows := len(view.renderMedia(msg, 40, base))

	if loadedRows < 2 {
		t.Fatalf("expected a multi-row image for 800x600 in 40 cols, got %d", loadedRows)
	}
	if loadingRows != loadedRows {
		t.Fatalf("placeholder rows %d != loaded rows %d — media load would shift the viewport", loadingRows, loadedRows)
	}
}

// An image of unknown source size cannot reserve height and falls back to a
// single spinner line (no crash, no over-reservation).
func TestLoadingPlaceholderUnknownSizeIsOneLine(t *testing.T) {
	st := store.New(0)
	url := "https://example.com/mystery"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "mystery.png", ContentType: "image/png", Size: 2048}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {loading: true}}

	if rows := len(view.renderMedia(st.Messages(1)[0], 40, view.styles.Cell("messages.content"))); rows != 1 {
		t.Fatalf("unknown-size placeholder = %d rows, want 1", rows)
	}
}
