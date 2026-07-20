package ui

import (
	"testing"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
	"github.com/diamondburned/arikawa/v3/session"
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

func TestSidebarUsesPingBadgesForChannelsAndServers(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general", Kind: store.ChannelText})
	a := app.New(discord.WrapSession(session.New("")), st, tui.New())
	a.SetActive(1, 10)
	st.IncrementPing(10)
	st.IncrementPing(10)
	mv := &MainView{app: a, state: &uistate.State{}, guildList: widget.NewItemList(nil), channelList: widget.NewItemList(nil)}
	mv.rebuildGuilds()
	mv.refreshChannels()
	if got := mv.guildList.Items()[0].Badge; got != "2" {
		t.Fatalf("server badge = %q, want 2", got)
	}
	if got := mv.channelList.Items()[0].Badge; got != "2" {
		t.Fatalf("channel badge = %q, want 2", got)
	}
}

func TestRefreshChannelsPreservesBrowsedSelection(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "active", Position: 1})
	st.UpsertChannel(store.Channel{ID: 20, GuildID: 1, Name: "browsing", Position: 2})
	a := app.New(discord.WrapSession(session.New("")), st, tui.New())
	a.SetActive(1, 10)
	mv := &MainView{app: a, state: &uistate.State{}, channelList: widget.NewItemList(nil)}
	mv.refreshChannels()
	mv.channelList.SetSelectedSilent(1)
	mv.refreshChannels()
	if got := mv.channelRows[mv.channelList.Selected()].ChannelID; got != 20 {
		t.Fatalf("selected channel after refresh = %d, want 20", got)
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

func TestOpenThreadMenuLabels(t *testing.T) {
	st := &uistate.State{}
	data := store.New(0)
	data.UpsertChannel(store.Channel{ID: 8, Kind: store.ChannelThread, Thread: &store.ThreadMeta{}})
	a := app.New(discord.WrapSession(session.New("")), data, tui.New())
	mv := &MainView{app: a, state: st, Root: widget.NewText("main")}
	sh := &Shell{app: a, cfg: config.Default(), mv: mv}

	sh.openThreadMenu(store.ChannelRow{ChannelID: 8, Kind: store.ChannelThread, Thread: true}, 2, 1)
	buf := tui.New().Render(sh, tui.Size{W: 40, H: 10})
	if !bufferContains(buf, "Pin thread") {
		t.Fatal("unpinned thread menu should offer Pin thread")
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
