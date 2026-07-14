package ui

import (
	"reflect"
	"testing"
	"time"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
)

func TestForumPostPromptSubmitsBodyAndMultipleTags(t *testing.T) {
	var title, body string
	var tags []uint64
	p := NewForumPostPrompt([]store.Tag{{ID: 1, Name: "bug"}, {ID: 2, Name: "help"}}, Styles{}, func(gotTitle, gotBody string, gotTags []uint64) {
		title, body, tags = gotTitle, gotBody, gotTags
	}, func() {})
	p.SetTitle("Crash")
	p.SetBody("Steps to reproduce")
	p.Handle(input.KeyEvent{Key: input.KeyTab})
	p.Handle(input.KeyEvent{Key: input.KeyTab})
	p.Handle(input.KeyEvent{Key: input.KeyRune, Rune: ' '})
	p.Handle(input.KeyEvent{Key: input.KeyDown})
	p.Handle(input.KeyEvent{Key: input.KeyRune, Rune: ' '})
	p.Handle(input.KeyEvent{Key: input.KeyEnter, Mods: input.Ctrl})
	if title != "Crash" || body != "Steps to reproduce" || !reflect.DeepEqual(tags, []uint64{1, 2}) {
		t.Fatalf("submit = %q, %q, %v", title, body, tags)
	}
}

func TestForumFilterRowOpensMenuAndSetFilter(t *testing.T) {
	forum := store.Channel{ID: 1, Kind: store.ChannelForum, Forum: &store.ForumMeta{Tags: []store.Tag{{ID: 7, Name: "bug"}}}}
	fv := NewForumView(Styles{}, false, nil, nil)
	opened := 0
	fv.onFilterMenu = func() { opened++ }
	fv.SetForum(forum, nil, nil, nil)
	fv.onSelect(0)
	if opened != 1 {
		t.Fatalf("filter menu opened %d times", opened)
	}
	fv.SetFilter(7)
	if fv.FilterTagID() != 7 {
		t.Fatalf("filter = %d, want 7", fv.FilterTagID())
	}
}

func TestForumViewNavigatesAdjacentForumsAtListEdges(t *testing.T) {
	forum := store.Channel{ID: 1, Kind: store.ChannelForum}
	fv := NewForumView(Styles{}, false, nil, nil)
	var delta int
	fv.onNavigate = func(got int) { delta = got }
	fv.SetForum(forum, nil, nil, nil)

	if !fv.Handle(input.KeyEvent{Key: input.KeyUp}) {
		t.Fatal("up at the first row should navigate forums")
	}
	if delta != -1 {
		t.Fatalf("up delta = %d, want -1", delta)
	}
	fv.list.SetSelectedSilent(len(fv.list.Items()) - 1)
	if !fv.Handle(input.KeyEvent{Key: input.KeyDown}) {
		t.Fatal("down at the last row should navigate forums")
	}
	if delta != 1 {
		t.Fatalf("down delta = %d, want 1", delta)
	}
}

func post(id store.ChannelID, name string, tags []uint64, replies int, active time.Time) store.Channel {
	return store.Channel{
		ID: id, Name: name, Kind: store.ChannelThread,
		Thread: &store.ThreadMeta{AppliedTags: tags, MessageCount: replies, LastActive: active},
	}
}

func TestForumPostLabel(t *testing.T) {
	now := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	tagByID := map[uint64]store.Tag{
		1: {ID: 1, Name: "bug"},
		2: {ID: 2, Name: "idea"},
	}
	p := post(10, "Crash on start", []uint64{1, 2}, 3, now.Add(-2*time.Hour))
	got := forumPostLabel(p, tagByID, now, false)
	want := "Crash on start ‹bug› ‹idea›  3 replies  · 2h"
	if got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

func TestForumPostLabelSingularAndNoTags(t *testing.T) {
	now := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	p := post(10, "Hi", nil, 1, now.Add(-30*time.Second))
	got := forumPostLabel(p, nil, now, true)
	want := "Hi  1 reply  · now"
	if got != want {
		t.Errorf("label = %q, want %q", got, want)
	}
}

func TestPostMatchesFilter(t *testing.T) {
	p := post(10, "x", []uint64{5, 6}, 0, time.Time{})
	if !postMatchesFilter(p, 0) {
		t.Error("filter 0 (all) should match everything")
	}
	if !postMatchesFilter(p, 5) {
		t.Error("post with tag 5 should match filter 5")
	}
	if postMatchesFilter(p, 9) {
		t.Error("post without tag 9 should not match filter 9")
	}
}

func TestSortForumPostsLatestActivity(t *testing.T) {
	base := time.Unix(1000, 0)
	posts := []store.Channel{
		post(1, "a", nil, 0, base.Add(1*time.Hour)),
		post(2, "b", nil, 0, base.Add(3*time.Hour)),
		post(3, "c", nil, 0, base.Add(2*time.Hour)),
	}
	got := sortForumPosts(posts, store.SortLatestActivity)
	want := []store.ChannelID{2, 3, 1}
	for i, w := range want {
		if got[i].ID != w {
			t.Errorf("activity sort[%d] = %d, want %d", i, got[i].ID, w)
		}
	}
}

func TestSortForumPostsCreationDate(t *testing.T) {
	posts := []store.Channel{
		post(1, "a", nil, 0, time.Unix(9000, 0)),
		post(3, "c", nil, 0, time.Unix(1, 0)),
		post(2, "b", nil, 0, time.Unix(5000, 0)),
	}
	got := sortForumPosts(posts, store.SortCreationDate)
	want := []store.ChannelID{3, 2, 1} // descending ID, ignoring activity
	for i, w := range want {
		if got[i].ID != w {
			t.Errorf("creation sort[%d] = %d, want %d", i, got[i].ID, w)
		}
	}
}

func TestRelTime(t *testing.T) {
	now := time.Unix(100000, 0)
	cases := []struct {
		then time.Time
		want string
	}{
		{time.Time{}, ""},
		{now.Add(-30 * time.Second), "now"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-3 * time.Hour), "3h"},
		{now.Add(-50 * time.Hour), "2d"},
	}
	for _, c := range cases {
		if got := relTime(c.then, now); got != c.want {
			t.Errorf("relTime(%v) = %q, want %q", c.then, got, c.want)
		}
	}
}
