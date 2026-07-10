package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
)

func TestChannelPrefixBadge(t *testing.T) {
	cases := map[store.ChannelKind]string{
		store.ChannelText:  "# ",
		store.ChannelVoice: "~ ",
		store.ChannelDM:    "",
	}
	for kind, want := range cases {
		if got := channelPrefixBadge(kind, false, false); got != want {
			t.Errorf("channelPrefixBadge(%v) = %q, want %q", kind, got, want)
		}
	}
}

func TestGuildItemFormatting(t *testing.T) {
	mv := &MainView{styles: Styles{}}
	cases := []struct {
		row  store.GuildRow
		want string
	}{
		{store.GuildRow{Folder: true, Name: "Work"}, glyphExpanded + " Work"},
		{store.GuildRow{Folder: true, Name: "Work", Collapsed: true}, glyphCollapsed + " Work"},
		{store.GuildRow{Name: "Den"}, "Den"},
		{store.GuildRow{Name: "Den", Indent: true}, "  Den"},
		{store.GuildRow{Name: "Den", Pinned: true}, glyphPinned + " Den"},
	}
	for _, c := range cases {
		if got := mv.guildItem(c.row).Label; got != c.want {
			t.Errorf("guildItem(%+v).Label = %q, want %q", c.row, got, c.want)
		}
	}
}

func TestOpenChannelMenuLabels(t *testing.T) {
	st := &uistate.State{}
	st.TogglePinnedChannel(5)
	mv := &MainView{state: st, Root: widget.NewText("main")}
	sh := &Shell{cfg: config.Default(), mv: mv}

	// Pinned channel → offers "Unpin channel".
	sh.openChannelMenu(store.ChannelRow{ChannelID: 5, Kind: store.ChannelText}, 2, 1)
	buf := tui.New().Render(sh, tui.Size{W: 40, H: 8})
	if !bufferContains(buf, "Unpin channel") {
		t.Fatal("pinned channel menu should offer Unpin")
	}
	sh.closePopup()

	// Unpinned channel → offers "Pin channel".
	sh.openChannelMenu(store.ChannelRow{ChannelID: 6, Kind: store.ChannelText}, 2, 1)
	buf = tui.New().Render(sh, tui.Size{W: 40, H: 8})
	if !bufferContains(buf, "Pin channel") {
		t.Fatal("unpinned channel menu should offer Pin")
	}
	sh.closePopup()

	// Collapsed category → offers "Expand".
	sh.openChannelMenu(store.ChannelRow{ChannelID: 7, Category: true, Collapsed: true}, 2, 1)
	buf = tui.New().Render(sh, tui.Size{W: 40, H: 8})
	if !bufferContains(buf, "Expand") {
		t.Fatal("collapsed category menu should offer Expand")
	}
}

func TestOpenGuildMenuLabels(t *testing.T) {
	st := &uistate.State{}
	mv := &MainView{state: st, Root: widget.NewText("main")}
	sh := &Shell{cfg: config.Default(), mv: mv}

	sh.openGuildMenu(store.GuildRow{GuildID: 9, Name: "Den"}, 2, 1)
	buf := tui.New().Render(sh, tui.Size{W: 40, H: 8})
	if !bufferContains(buf, "Pin server") {
		t.Fatal("unpinned guild menu should offer Pin server")
	}
	sh.closePopup()

	sh.openGuildMenu(store.GuildRow{Folder: true, Name: "Work"}, 2, 1)
	buf = tui.New().Render(sh, tui.Size{W: 40, H: 8})
	if !bufferContains(buf, "Collapse") {
		t.Fatal("expanded folder menu should offer Collapse")
	}
}
