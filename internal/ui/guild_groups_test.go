package ui

import (
	"reflect"
	"testing"

	"awesomeProject/internal/app"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
	"github.com/diamondburned/arikawa/v3/session"
)

func groupIDs(groups []store.GuildFolder) [][]store.GuildID {
	out := make([][]store.GuildID, len(groups))
	for i, group := range groups {
		out[i] = group.GuildIDs
	}
	return out
}

func TestCleanGroupsAddsMissingGuilds(t *testing.T) {
	groups := []store.GuildFolder{{ID: 10, GuildIDs: []store.GuildID{1, 9}}}
	guilds := []store.Guild{{ID: 1}, {ID: 2}, {ID: app.DirectMessagesGuildID}}
	got := cleanGroups(groups, guilds)
	want := [][]store.GuildID{{1}, {2}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("cleanGroups = %v, want %v", groupIDs(got), want)
	}
}

func TestMoveGuildToTop(t *testing.T) {
	groups := []store.GuildFolder{
		{ID: 10, GuildIDs: []store.GuildID{1, 2}},
		{GuildIDs: []store.GuildID{3}},
	}
	got := moveGuild(groups, 2, store.GuildRow{GuildID: app.DirectMessagesGuildID}, false)
	want := [][]store.GuildID{{2}, {1}, {3}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("moveGuild = %v, want %v", groupIDs(got), want)
	}
}

func TestMoveGuildIntoGroup(t *testing.T) {
	groups := []store.GuildFolder{
		{ID: 10, GuildIDs: []store.GuildID{1, 2}},
		{GuildIDs: []store.GuildID{3}},
	}
	got := moveGuild(groups, 3, store.GuildRow{Folder: true, FolderID: 10}, false)
	want := [][]store.GuildID{{1, 2, 3}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("moveGuild = %v, want %v", groupIDs(got), want)
	}
}

func TestMoveGuildToOwnGroupHeaderCommitsPreviewOrder(t *testing.T) {
	groups := []store.GuildFolder{{ID: 10, GuildIDs: []store.GuildID{1, 2, 3}}}
	got := moveGuild(groups, 1, store.GuildRow{Folder: true, FolderID: 10}, false)
	want := [][]store.GuildID{{2, 3, 1}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("moveGuild = %v, want %v", groupIDs(got), want)
	}
}

func TestMoveGuildBeforeGroupMember(t *testing.T) {
	groups := []store.GuildFolder{
		{ID: 10, GuildIDs: []store.GuildID{1, 2}},
		{GuildIDs: []store.GuildID{3}},
	}
	got := moveGuild(groups, 3, store.GuildRow{GuildID: 2, FolderID: 10}, false)
	want := [][]store.GuildID{{1, 3, 2}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("moveGuild = %v, want %v", groupIDs(got), want)
	}
}

func TestMoveGuildAfterGroupMember(t *testing.T) {
	groups := []store.GuildFolder{{ID: 10, GuildIDs: []store.GuildID{1, 2, 3}}}
	got := moveGuild(groups, 1, store.GuildRow{GuildID: 2, FolderID: 10}, true)
	want := [][]store.GuildID{{2, 1, 3}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("moveGuild = %v, want %v", groupIDs(got), want)
	}
}

func TestMakeGroupKeepsPosition(t *testing.T) {
	groups := []store.GuildFolder{
		{ID: 10, GuildIDs: []store.GuildID{1, 2}},
		{GuildIDs: []store.GuildID{3}},
	}
	got := makeGroup(groups, 2, "Work")
	want := [][]store.GuildID{{1}, {2}, {3}}
	if !reflect.DeepEqual(groupIDs(got), want) {
		t.Fatalf("makeGroup = %v, want %v", groupIDs(got), want)
	}
	if got[1].ID != -1 || got[1].Name != "Work" {
		t.Fatalf("new group = %+v", got[1])
	}
}

func TestCreateGroupPersistsLayout(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	data := store.New(0)
	data.UpsertGuild(store.Guild{ID: 1, Name: "Alpha"})
	data.UpsertGuild(store.Guild{ID: 2, Name: "Beta"})
	data.SetGuildFolders([]store.GuildFolder{
		{GuildIDs: []store.GuildID{1}},
		{GuildIDs: []store.GuildID{2}},
	})
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{Key: "acct", ID: 42}}}}
	a := app.New(discord.WrapSession(session.New("")), data, tui.New())
	mv := &MainView{app: a, state: state, guildList: widget.NewItemList(nil)}

	mv.CreateGroup(2, "Games")

	layout, ok := state.GuildLayout("acct", 42)
	if !ok || len(layout) != 2 || layout[1].Name != "Games" || !reflect.DeepEqual(layout[1].GuildIDs, []uint64{2}) {
		t.Fatalf("saved layout = %+v,%v", layout, ok)
	}
	if len(mv.guildRows) != 3 || !mv.guildRows[1].Folder || mv.guildRows[1].Name != "Games" {
		t.Fatalf("rendered rows = %+v", mv.guildRows)
	}
}

func TestDropGuildExpandsTargetGroup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	data := store.New(0)
	data.UpsertGuild(store.Guild{ID: 1, Name: "Alpha"})
	data.UpsertGuild(store.Guild{ID: 2, Name: "Beta"})
	data.SetGuildFolders([]store.GuildFolder{
		{ID: 10, Name: "Work", GuildIDs: []store.GuildID{1}},
		{GuildIDs: []store.GuildID{2}},
	})
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{Key: "acct", ID: 42}}}}
	state.ToggleCollapsedFolder("acct", 42, 10)
	a := app.New(discord.WrapSession(session.New("")), data, tui.New())
	mv := &MainView{app: a, state: state, guildList: widget.NewItemList(nil)}
	mv.rebuildGuilds()

	mv.dropGuild(1, 0)

	layout, ok := state.GuildLayout("acct", 42)
	if !ok || len(layout) != 1 || !reflect.DeepEqual(layout[0].GuildIDs, []uint64{1, 2}) {
		t.Fatalf("saved layout = %+v,%v", layout, ok)
	}
	if state.IsFolderCollapsed("acct", 42, 10) {
		t.Fatal("target group stayed collapsed")
	}
	if len(mv.guildRows) != 3 || mv.guildRows[2].GuildID != 2 {
		t.Fatalf("rendered rows = %+v", mv.guildRows)
	}
}

func TestMainViewScopesSameNegativeFolderIDByAccount(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	firstStore := store.New(0)
	firstStore.UpsertGuild(store.Guild{ID: 1, Name: "Alpha"})
	firstStore.SetGuildFolders([]store.GuildFolder{{ID: -1, Name: "Local A", GuildIDs: []store.GuildID{1}}})
	secondStore := store.New(0)
	secondStore.UpsertGuild(store.Guild{ID: 2, Name: "Beta"})
	secondStore.SetGuildFolders([]store.GuildFolder{{ID: -1, Name: "Local B", GuildIDs: []store.GuildID{2}}})
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{Key: "acct-a"}, {Key: "acct-b"}}}}
	firstApp := app.New(discord.WrapSession(session.New("")), firstStore, tui.New())
	secondApp := app.New(discord.WrapSession(session.New("")), secondStore, tui.New())
	mv := &MainView{app: firstApp, state: state, guildList: widget.NewItemList(nil)}

	mv.ToggleCollapseFolder(-1)
	if !state.IsFolderCollapsed("acct-a", 0, -1) || state.IsFolderCollapsed("acct-b", 0, -1) {
		t.Fatalf("first collapse leaked between accounts: %+v", state.GuildLayouts)
	}

	state.Accounts.Active = 1
	mv.app = secondApp
	mv.rebuildGuilds()
	if len(mv.guildRows) != 2 || mv.guildRows[0].Collapsed {
		t.Fatalf("second account inherited first collapse: %+v", mv.guildRows)
	}
	mv.ToggleCollapseFolder(-1)
	state.Accounts.Active = 0
	mv.app = firstApp
	mv.ToggleCollapseFolder(-1)
	if state.IsFolderCollapsed("acct-a", 0, -1) || !state.IsFolderCollapsed("acct-b", 0, -1) {
		t.Fatalf("independent toggles collided: %+v", state.GuildLayouts)
	}
}

func TestMainViewKeepsTwoUnhydratedLayoutsByAccountKey(t *testing.T) {
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{Key: "acct-a"}, {Key: "acct-b"}}}}
	firstStore := store.New(0)
	firstStore.UpsertGuild(store.Guild{ID: 1, Name: "Alpha"})
	secondStore := store.New(0)
	secondStore.UpsertGuild(store.Guild{ID: 2, Name: "Beta"})
	firstApp := app.New(discord.WrapSession(session.New("")), firstStore, tui.New())
	secondApp := app.New(discord.WrapSession(session.New("")), secondStore, tui.New())
	mv := &MainView{app: firstApp, state: state}
	mv.saveGroups([]store.GuildFolder{{ID: -1, Name: "First", GuildIDs: []store.GuildID{1}}})

	state.Accounts.Active = 1
	mv.app = secondApp
	mv.saveGroups([]store.GuildFolder{{ID: -1, Name: "Second", GuildIDs: []store.GuildID{2}}})
	if len(state.GuildLayouts) != 2 {
		t.Fatalf("unhydrated AccountID-zero layouts collided: %+v", state.GuildLayouts)
	}
	first, okFirst := state.GuildLayout("acct-a", 0)
	second, okSecond := state.GuildLayout("acct-b", 0)
	if !okFirst || !okSecond || first[0].Name != "First" || second[0].Name != "Second" {
		t.Fatalf("saved keyed layouts = %+v / %+v", first, second)
	}

	state.Accounts.Active = 0
	state.Accounts.List[0].ID = 77
	mv.app = firstApp
	groups := mv.currentGroups()
	if len(groups) != 1 || groups[0].Name != "First" || state.GuildLayouts[0].AccountID != 77 {
		t.Fatalf("first layout disappeared after ID hydration: groups=%+v layouts=%+v", groups, state.GuildLayouts)
	}
}

func TestDragGuildPlacesPreviewAfterGroup(t *testing.T) {
	mv := &MainView{guildRows: []store.GuildRow{
		{GuildID: app.DirectMessagesGuildID},
		{GuildID: 3},
		{Folder: true, FolderID: 10},
		{GuildID: 1, FolderID: 10, Indent: true},
		{GuildID: 2, FolderID: 10, Indent: true},
	}}
	if got := mv.dragGuild(1, 2); got != 4 {
		t.Fatalf("dragGuild = %d, want 4", got)
	}
	if got := mv.dragGuild(4, 0); got != 1 {
		t.Fatalf("dragGuild over Direct Messages = %d, want 1", got)
	}
}
