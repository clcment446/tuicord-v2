package ui

import (
	"testing"
	"time"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
)

// TestForumViewRenders80x24 exercises the forum post-list view at the minimum
// supported terminal size, in both Unicode and ASCII modes, to catch panics and
// width blowups.
func TestForumViewRenders80x24(t *testing.T) {
	now := time.Now()
	forum := store.Channel{ID: 1, Name: "help", Kind: store.ChannelForum,
		Forum: &store.ForumMeta{Tags: []store.Tag{{ID: 1, Name: "bug"}, {ID: 2, Name: "idea"}}}}
	active := []store.Channel{
		post(10, "Cannot log in on the new build after update", []uint64{1}, 12, now.Add(-time.Hour)),
		post(11, "Idea: dark mode for the settings pane", []uint64{2}, 0, now.Add(-3*time.Hour)),
	}
	archived := []store.Channel{post(12, "Old resolved report", nil, 4, now.Add(-100*time.Hour))}

	for _, ascii := range []bool{false, true} {
		fv := NewForumView(Styles{}, ascii, func(store.ChannelID) {}, func(store.ChannelID) {})
		fv.SetForum(forum, active, archived, func(store.ChannelID) int { return 0 })
		buf := tui.New().Render(fv, tui.Size{W: 80, H: 24})
		if !bufferContains(buf, "help") && !bufferContains(buf, "Cannot log in") {
			t.Errorf("ascii=%v: forum view did not render posts", ascii)
		}
		if !bufferContains(buf, "Load archived") {
			t.Errorf("ascii=%v: forum view missing archived loader footer", ascii)
		}
	}
}

// TestForumTagFilterCycle checks the tag filter cycles all → tags → all and
// narrows the visible posts.
func TestForumTagFilterCycle(t *testing.T) {
	forum := store.Channel{ID: 1, Name: "help", Kind: store.ChannelForum,
		Forum: &store.ForumMeta{Tags: []store.Tag{{ID: 1, Name: "bug"}, {ID: 2, Name: "idea"}}}}
	fv := NewForumView(Styles{}, false, func(store.ChannelID) {}, func(store.ChannelID) {})
	fv.SetForum(forum, nil, nil, nil)

	if fv.FilterTagID() != 0 {
		t.Fatalf("initial filter = %d, want 0 (all)", fv.FilterTagID())
	}
	fv.cycleFilter()
	if fv.FilterTagID() != 1 {
		t.Errorf("after 1 cycle filter = %d, want tag 1", fv.FilterTagID())
	}
	fv.cycleFilter()
	if fv.FilterTagID() != 2 {
		t.Errorf("after 2 cycles filter = %d, want tag 2", fv.FilterTagID())
	}
	fv.cycleFilter()
	if fv.FilterTagID() != 0 {
		t.Errorf("after 3 cycles filter = %d, want back to all", fv.FilterTagID())
	}
}

// TestPromptConfirm verifies the name prompt trims and submits non-empty input
// and cancels on empty.
func TestPromptConfirm(t *testing.T) {
	var got string
	closed := false
	p := NewPrompt("New thread", "name", Styles{},
		func(v string) { got = v },
		func() { closed = true })
	p.input.SetValue("  my thread  ")
	p.confirm()
	if got != "my thread" {
		t.Errorf("submitted %q, want trimmed 'my thread'", got)
	}
	if !closed {
		t.Error("prompt should close after confirm")
	}

	got, closed = "", false
	p2 := NewPrompt("t", "n", Styles{}, func(v string) { got = v }, func() { closed = true })
	p2.input.SetValue("   ")
	p2.confirm()
	if got != "" {
		t.Errorf("empty input should not submit, got %q", got)
	}
	if !closed {
		t.Error("empty confirm should still close")
	}
}
