package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
)

func findRow(buf *screen.Buffer, needle string) int {
	for y := 0; y < buf.Height(); y++ {
		if strings.Contains(rowText(buf, y), needle) {
			return y
		}
	}
	return -1
}

// Folding and unfolding a markdown header must keep the header line pinned at
// the screen row it was clicked on — repeated cycles previously crept the
// viewport upward (#28).
func TestHeaderFoldKeepsToggledRowStable(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, AuthorID: 1, Author: "alice", Content: "x1\nx2\nx3\nx4\nx5\nx6"})
	st.AppendMessage(store.Message{ID: 2, ChannelID: 1, AuthorID: 2, Author: "bob", Content: "# Sec\ns1\ns2\ns3\ns4"})
	st.AppendMessage(store.Message{ID: 3, ChannelID: 1, AuthorID: 3, Author: "carol", Content: "c1\nc2\nc3\nc4\nc5\nc6"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	// Scroll up from the bottom just until the header is visible, so the
	// viewport sits mid-transcript with foldable content below and plenty of
	// lines above — the position where fold/unfold used to shift the view.
	headerRow := -1
	for offset := 1; offset < 64 && headerRow < 0; offset++ {
		view.bottomScroll.SetOffset(offset)
		view.Draw(buf.Clip(buf.Bounds()))
		headerRow = findRow(buf, "Sec")
	}
	if headerRow < 0 {
		t.Fatal("header never became visible while scrolling up")
	}

	for cycle := 0; cycle < 3; cycle++ {
		// Click the fold glyph (col < 2) on the header's row, then redraw.
		view.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: 0, Y: headerRow})
		view.Draw(buf.Clip(buf.Bounds()))
		if got := findRow(buf, "Sec"); got != headerRow {
			t.Fatalf("cycle %d: header row = %d, want pinned at %d", cycle, got, headerRow)
		}
	}
}

// Expanding and collapsing a v2 list control (select) must keep the control
// chip pinned at its screen row across repeated toggles (#28).
func TestComponentListToggleKeepsControlRowStable(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, AuthorID: 1, Author: "bob", Content: "above1\nabove2\nabove3"})
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 2, Author: "carol",
		Flags: 1 << 15, // IsComponentsV2
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentSelect, CustomID: "sel", Placeholder: "PickMe",
			Options: []store.ComponentOption{
				{Label: "One", Value: "1"}, {Label: "Two", Value: "2"},
				{Label: "Three", Value: "3"}, {Label: "Four", Value: "4"},
			},
		}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(40, 6)
	view.Draw(buf.Clip(buf.Bounds()))

	controlRow := -1
	controlX := 0
	for row, line := range view.visibleLines {
		if len(line.actions) > 0 {
			controlRow = row
			controlX = line.actions[0].start
			break
		}
	}
	if controlRow < 0 {
		t.Fatal("select control not visible")
	}

	for cycle := 0; cycle < 3; cycle++ {
		view.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: controlX, Y: controlRow})
		view.Draw(buf.Clip(buf.Bounds()))
		row := -1
		for r, line := range view.visibleLines {
			for _, hit := range line.actions {
				if hit.action.customID == "sel" {
					row = r
				}
			}
			if row >= 0 {
				break
			}
		}
		if row != controlRow {
			t.Fatalf("cycle %d: control row = %d, want pinned at %d", cycle, row, controlRow)
		}
	}
}

// An embed image whose source dimensions Discord reported must reserve its
// final height while loading, so the load cannot shift the viewport (#28/#29).
func TestEmbedLoadingPlaceholderReservesImageHeight(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/pic.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Embeds: []store.Embed{{Kind: store.EmbedImage, ImageURL: url, ImageW: 800, ImageH: 600}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	base := view.styles.Cell("messages.content")
	msg := st.Messages(1)[0]

	view.media = map[string]*chatMediaState{url: {loading: true}}
	loadingRows := len(view.renderEmbeds(msg, 40, base))

	view.media = map[string]*chatMediaState{url: {img: solidTestImage(80, 60)}}
	loadedRows := len(view.renderEmbeds(msg, 40, base))

	if loadedRows < 2 {
		t.Fatalf("expected a multi-row embed image, got %d rows", loadedRows)
	}
	if loadingRows != loadedRows {
		t.Errorf("loading rows = %d, loaded rows = %d — height changes on load", loadingRows, loadedRows)
	}
}

// Video posters and non-animated GIF stills append a trailer text row below
// the image. The loading placeholder must include that row, or every video and
// GIF grows one line on load.
func TestLoadingPlaceholderIncludesMediaTrailerRow(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	cfg := media.DefaultConfig()
	cfg.Animate = false
	view.SetMedia(nil, cfg, nil)
	base := view.styles.Cell("messages.content")
	spec := mediaSpec{maxCols: 40, maxRows: 12, sourceW: 800, sourceH: 600}

	for _, url := range []string{
		"https://cdn.discordapp.com/attachments/1/2/clip.mp4",
		"https://cdn.discordapp.com/attachments/1/2/anim.gif",
	} {
		view.media = map[string]*chatMediaState{url: {loading: true}}
		loading := len(view.mediaLines(url, "chip", "key", base, spec, false))
		view.media = map[string]*chatMediaState{url: {img: solidTestImage(80, 60)}}
		loaded := len(view.mediaLines(url, "chip", "key", base, spec, false))
		if loading != loaded {
			t.Errorf("%s: loading rows = %d, loaded rows = %d", url, loading, loaded)
		}
	}
}
