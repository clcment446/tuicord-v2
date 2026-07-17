package ui

import (
	"fmt"
	"strings"
	"time"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// forumTargetKind distinguishes the two selectable row kinds in a forum view:
// a post (open it) and the archived-loader footer.
type forumTargetKind int

const (
	forumTargetPost forumTargetKind = iota
	forumTargetLoadArchived
	forumTargetFilter
)

type forumTarget struct {
	kind    forumTargetKind
	channel store.ChannelID
}

// ForumView renders a forum channel's posts as a selectable list instead of a
// chat: one row per thread (post) with tag chips, reply count, and last-active,
// plus an optional "Load archived…" footer. It is embedded in the chat column
// (not an overlay). Selecting a post opens it as the active channel; the footer
// paginates archived posts.
type ForumView struct {
	header  *widget.Text
	list    *widget.ItemList
	body    tui.Widget
	node    layout.Node
	styles  Styles
	ascii   bool
	forum   store.Channel
	targets []forumTarget

	// filterIdx is 0 for "all", else 1+index into forum.Forum.Tags.
	filterIdx int

	onOpen         func(store.ChannelID)
	onLoadArchived func(store.ChannelID)
	onNavigate     func(int)
	// onFilterCycle is invoked after the tag filter changes so the owner can
	// rebuild the row list from its post slices.
	onFilterCycle func()
	onFilterMenu  func()
	onPreview     func(store.ChannelID)
}

// NewForumView builds an empty forum view. onOpen fires for a post row; the
// archived-loader footer calls onLoadArchived.
func NewForumView(styles Styles, ascii bool, onOpen func(store.ChannelID), onLoadArchived func(store.ChannelID)) *ForumView {
	fv := &ForumView{
		header:         widget.NewText(""),
		list:           widget.NewItemList(nil),
		styles:         styles,
		ascii:          ascii,
		onOpen:         onOpen,
		onLoadArchived: onLoadArchived,
		node:           layout.Node{Grow: 1},
	}
	fv.header.SetStyle(styles.Cell("forum.header"))
	fv.header.SetWrap(false)
	fv.list.SetStyle(styles.Cell("forum.body"))
	fv.list.SetSelectedStyle(styles.Cell("forum.selected"))
	fv.list.SetBadgeStyle(styles.Cell("forum.badge"))
	fv.list.OnSelect(fv.onSelect)
	fv.setBody(nil)
	return fv
}

func (fv *ForumView) setBody(preview tui.Widget) {
	left := widget.Column(fv.header, fv.list)
	left.Children()[0].Layout().Basis = 1
	left.Children()[0].Layout().Grow = 0
	left.Children()[1].Layout().Grow = 1
	if preview == nil {
		fv.body = left
		return
	}
	fv.body = widget.NewSplit(left, preview).Basis(34).MinFirst(18).MinSecond(20).Vertical()
}

// SetPreview installs the right-hand pane used to show the selected post.
func (fv *ForumView) SetPreview(preview tui.Widget) { fv.setBody(preview) }

// SetForum installs the forum and its posts. active are the live posts; archived
// are the paginated archived posts already loaded (may be empty). unread reports
// a post's unread count for the badge.
func (fv *ForumView) SetForum(forum store.Channel, active, archived []store.Channel, unread func(store.ChannelID) int) {
	fv.forum = forum
	fv.rebuild(active, archived, unread)
}

// activeFilter returns the currently selected tag filter, or ok=false for "all".
func (fv *ForumView) activeFilter() (store.Tag, bool) {
	if fv.forum.Forum == nil || fv.filterIdx <= 0 || fv.filterIdx > len(fv.forum.Forum.Tags) {
		return store.Tag{}, false
	}
	return fv.forum.Forum.Tags[fv.filterIdx-1], true
}

// FilterTagID returns the active filter tag's ID, or 0 for "all". New posts
// created from this view inherit it.
func (fv *ForumView) FilterTagID() uint64 {
	if t, ok := fv.activeFilter(); ok {
		return t.ID
	}
	return 0
}

func (fv *ForumView) rebuild(active, archived []store.Channel, unread func(store.ChannelID) int) {
	sort := store.SortLatestActivity
	var tags []store.Tag
	if fv.forum.Forum != nil {
		sort = fv.forum.Forum.DefaultSort
		tags = fv.forum.Forum.Tags
	}
	tagByID := make(map[uint64]store.Tag, len(tags))
	for _, t := range tags {
		tagByID[t.ID] = t
	}
	filterID := fv.FilterTagID()
	now := time.Now()

	active = sortForumPosts(active, sort)
	archived = sortForumPosts(archived, sort)

	items := make([]widget.Item, 0, len(active)+len(archived)+3)
	fv.targets = fv.targets[:0]
	filterName := "all"
	if tag, ok := fv.activeFilter(); ok {
		filterName = tag.Name
	}
	items = append(items, widget.Item{Label: "Filter tags… [" + filterName + "]", Style: fv.styles.Cell("forum.filter")})
	fv.targets = append(fv.targets, forumTarget{kind: forumTargetFilter})
	addPost := func(p store.Channel, dim bool) {
		if !postMatchesFilter(p, filterID) {
			return
		}
		style := fv.styles.Cell("forum.body")
		if dim {
			style = fv.styles.Cell("forum.archived")
		}
		badge := ""
		if unread != nil {
			badge = unreadBadge(unread(p.ID))
		}
		items = append(items, widget.Item{Label: forumPostLabel(p, tagByID, now, fv.ascii), Badge: badge, Style: style})
		fv.targets = append(fv.targets, forumTarget{kind: forumTargetPost, channel: p.ID})
	}
	for _, p := range active {
		addPost(p, false)
	}
	if len(archived) > 0 {
		for _, p := range archived {
			addPost(p, true)
		}
	}
	// Footer: load more archived posts.
	loadLabel := "＋ Load archived…"
	if fv.ascii {
		loadLabel = "+ Load archived..."
	}
	items = append(items, widget.Item{Label: loadLabel, Style: fv.styles.Cell("forum.archived")})
	fv.targets = append(fv.targets, forumTarget{kind: forumTargetLoadArchived, channel: fv.forum.ID})

	fv.list.SetItems(items)
	fv.updateHeader(len(active))
}

func (fv *ForumView) updateHeader(count int) {
	filter := "all"
	if t, ok := fv.activeFilter(); ok {
		filter = t.Name
	}
	fv.header.SetContent(fmt.Sprintf("%d posts · filter: %s   [f] cycle tag", count, filter))
}

func (fv *ForumView) onSelect(index int) {
	if index < 0 || index >= len(fv.targets) {
		return
	}
	t := fv.targets[index]
	switch t.kind {
	case forumTargetPost:
		if fv.onPreview != nil {
			fv.onPreview(t.channel)
		}
		if fv.onOpen != nil {
			fv.onOpen(t.channel)
		}
	case forumTargetLoadArchived:
		if fv.onLoadArchived != nil {
			fv.onLoadArchived(t.channel)
		}
	case forumTargetFilter:
		if fv.onFilterMenu != nil {
			fv.onFilterMenu()
		}
	}
}

// SetFilter selects a tag by ID, or all posts when id is zero.
func (fv *ForumView) SetFilter(id uint64) {
	fv.filterIdx = 0
	if fv.forum.Forum != nil {
		for i, tag := range fv.forum.Forum.Tags {
			if tag.ID == id {
				fv.filterIdx = i + 1
				break
			}
		}
	}
	if fv.onFilterCycle != nil {
		fv.onFilterCycle()
	}
}

// cycleFilter advances the tag filter to the next available tag (wrapping back
// to "all"). It reports whether a rebuild is needed by the caller.
func (fv *ForumView) cycleFilter() {
	n := 0
	if fv.forum.Forum != nil {
		n = len(fv.forum.Forum.Tags)
	}
	if n == 0 {
		return
	}
	fv.filterIdx = (fv.filterIdx + 1) % (n + 1)
}

// Children exposes the composed body.
func (fv *ForumView) Children() []tui.Widget { return []tui.Widget{fv.body} }

// Measure delegates to the body.
func (fv *ForumView) Measure(avail tui.Size) tui.Size { return fv.body.Measure(avail) }

// Layout returns the forum-view layout node.
func (fv *ForumView) Layout() *layout.Node {
	fv.node.Children = []*layout.Node{fv.body.Layout()}
	return &fv.node
}

// Draw is a no-op; children draw themselves.
func (fv *ForumView) Draw(screen.Region) {}

// CanFocus lets the forum view take keyboard focus for list navigation.
func (fv *ForumView) CanFocus() bool { return true }

// filterChanged is set by Handle so the container can rebuild the list.
func (fv *ForumView) Handle(ev tui.Event) bool {
	if key, ok := ev.(input.KeyEvent); ok && !key.Release {
		if key.Key == input.KeyRune && (key.Rune == 'f' || key.Rune == 'F') {
			if fv.onFilterMenu != nil {
				fv.onFilterMenu()
			}
			return true
		}
		if fv.onNavigate != nil {
			switch key.Key {
			case input.KeyUp:
				if fv.list.Selected() <= 0 {
					fv.onNavigate(-1)
					return true
				}
			case input.KeyDown:
				if fv.list.Selected() >= len(fv.list.Items())-1 {
					fv.onNavigate(1)
					return true
				}
			}
		}
	}
	return fv.list.Handle(ev)
}

// forumPostLabel renders one forum-post row label: title, tag chips, reply
// count, and a short relative last-active time. It is pure so the row shape is
// table-testable given a fixed now.
func forumPostLabel(post store.Channel, tagByID map[uint64]store.Tag, now time.Time, ascii bool) string {
	var b strings.Builder
	b.WriteString(post.Name)
	if post.Thread == nil {
		return b.String()
	}
	for _, id := range post.Thread.AppliedTags {
		t, ok := tagByID[id]
		if !ok {
			continue
		}
		b.WriteString(" ")
		b.WriteString(tagChip(t, ascii))
	}
	if n := post.Thread.MessageCount; n > 0 {
		fmt.Fprintf(&b, "  %d %s", n, plural(n, "reply", "replies"))
	}
	if rt := relTime(post.Thread.LastActive, now); rt != "" {
		b.WriteString("  · ")
		b.WriteString(rt)
	}
	return b.String()
}

func tagChip(t store.Tag, ascii bool) string {
	name := t.Name
	if t.Emoji != "" && !ascii {
		name = t.Emoji + " " + name
	}
	if ascii {
		return "<" + name + ">"
	}
	return "‹" + name + "›"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// relTime formats a coarse relative time ("now", "5m", "3h", "2d"). A zero time
// yields the empty string.
func relTime(then, now time.Time) string {
	if then.IsZero() {
		return ""
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// postMatchesFilter reports whether a post carries the tag filter (0 = all).
func postMatchesFilter(post store.Channel, tagID uint64) bool {
	if tagID == 0 {
		return true
	}
	if post.Thread == nil {
		return false
	}
	for _, id := range post.Thread.AppliedTags {
		if id == tagID {
			return true
		}
	}
	return false
}

// sortForumPosts orders posts by the forum's default sort: most-recent activity,
// or newest-created (descending ID) for the creation-date sort.
func sortForumPosts(posts []store.Channel, sort store.ThreadSort) []store.Channel {
	out := append([]store.Channel(nil), posts...)
	less := func(a, b store.Channel) bool {
		if sort == store.SortCreationDate {
			return a.ID > b.ID
		}
		at, bt := activeAt(a), activeAt(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
		return a.ID > b.ID
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && less(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func activeAt(c store.Channel) time.Time {
	if c.Thread == nil {
		return time.Time{}
	}
	return c.Thread.LastActive
}
