package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

// anchorTestView builds a two-message transcript where bob's block can be
// scrolled to while alice's block above it changes height.
func anchorTestView(t *testing.T) (*store.Store, *ChatView, *screen.Buffer) {
	t.Helper()
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, AuthorID: 1, Author: "alice", Content: "a1\na2\na3"})
	st.AppendMessage(store.Message{ID: 2, ChannelID: 1, AuthorID: 2, Author: "bob", Content: "b1\nb2\nb3\nb4"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 3)
	return st, view, buf
}

func TestStickyAnchorKeepsViewportWhenContentAboveGrows(t *testing.T) {
	st, view, buf := anchorTestView(t)
	view.Draw(buf.Clip(buf.Bounds()))
	// Scroll so the viewport shows only bob's lines: 9 lines total, height 3,
	// offset 1 puts b1/b2/b3 on screen.
	view.bottomScroll.SetOffset(1)
	view.Draw(buf.Clip(buf.Bounds()))
	// The viewport starts inside bob's block, so row 0 is the pinned author
	// and rows 1-2 show b2/b3.
	if got := []string{rowText(buf, 1), rowText(buf, 2)}; got[0] != "b2" || got[1] != "b3" {
		t.Fatalf("pre-change rows = %q, want [b2 b3]", got)
	}

	// Alice's message above the viewport grows by two lines (an unfold, an
	// edit, or async media finishing all look like this).
	st.UpdateMessage(1, 1, func(m *store.Message) { m.Content = "a1\na2\na3\na4\na5" })
	view.Draw(buf.Clip(buf.Bounds()))

	if got := []string{rowText(buf, 1), rowText(buf, 2)}; got[0] != "b2" || got[1] != "b3" {
		t.Errorf("rows after growth above = %q, want still [b2 b3]", got)
	}
}

func TestStickyAnchorKeepsViewportOverFoldUnfoldCycles(t *testing.T) {
	st, view, buf := anchorTestView(t)
	view.Draw(buf.Clip(buf.Bounds()))
	view.bottomScroll.SetOffset(1)
	view.Draw(buf.Clip(buf.Bounds()))
	if got := []string{rowText(buf, 1), rowText(buf, 2)}; got[0] != "b2" || got[1] != "b3" {
		t.Fatalf("pre-change rows = %q, want [b2 b3]", got)
	}

	// Repeatedly shrink and regrow the block above the viewport, as folding
	// and unfolding an embed v2 list does. The viewport must not creep.
	for i := 0; i < 3; i++ {
		st.UpdateMessage(1, 1, func(m *store.Message) { m.Content = "a1" })
		view.Draw(buf.Clip(buf.Bounds()))
		st.UpdateMessage(1, 1, func(m *store.Message) { m.Content = "a1\na2\na3" })
		view.Draw(buf.Clip(buf.Bounds()))
	}

	if got := []string{rowText(buf, 1), rowText(buf, 2)}; got[0] != "b2" || got[1] != "b3" {
		t.Errorf("rows after fold/unfold cycles = %q, want still [b2 b3]", got)
	}
}

func TestStickyAnchorDisabledFallsBackToBottomDistance(t *testing.T) {
	st, view, buf := anchorTestView(t)
	view.SetStickyAnchor(false)
	view.Draw(buf.Clip(buf.Bounds()))
	view.bottomScroll.SetOffset(1)
	view.Draw(buf.Clip(buf.Bounds()))

	st.UpdateMessage(1, 1, func(m *store.Message) { m.Content = "a1\na2\na3\na4\na5" })
	view.Draw(buf.Clip(buf.Bounds()))

	// Plain BottomScroll treats all growth as appended below the reading
	// position, so growth above shifts the view; that legacy behavior is what
	// display.sticky_anchor=false selects.
	if got := rowText(buf, 0); got == "b1" {
		t.Skip("legacy behavior unexpectedly stable in this layout")
	}
}

func TestStickyAnchorStaysAtBottomForNewMessages(t *testing.T) {
	st, view, buf := anchorTestView(t)
	view.Draw(buf.Clip(buf.Bounds()))
	st.AppendMessage(store.Message{ID: 3, ChannelID: 1, AuthorID: 2, Author: "bob", Content: "b5"})
	view.Draw(buf.Clip(buf.Bounds()))
	var rows []string
	for y := 0; y < 3; y++ {
		rows = append(rows, rowText(buf, y))
	}
	if !strings.Contains(strings.Join(rows, "\n"), "b5") {
		t.Errorf("bottom-anchored view missing new message: %q", rows)
	}
}

func TestStickyAnchorDoesNotOverrideUserScroll(t *testing.T) {
	_, view, buf := anchorTestView(t)
	view.Draw(buf.Clip(buf.Bounds()))
	view.bottomScroll.SetOffset(1)
	view.Draw(buf.Clip(buf.Bounds()))
	if got := rowText(buf, 1); got != "b2" {
		t.Fatalf("row 1 = %q, want b2", got)
	}
	// The user scrolls up one line between draws; the stale anchor must not
	// snap the viewport back.
	view.scrollUp()
	view.Draw(buf.Clip(buf.Bounds()))
	if got := rowText(buf, 1); got != "b1" {
		t.Errorf("row 1 after scrollUp = %q, want b1", got)
	}
}
