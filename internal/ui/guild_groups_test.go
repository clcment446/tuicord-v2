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
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{ID: 42}}}}
	a := app.New(discord.WrapSession(session.New("")), data, tui.New())
	mv := &MainView{app: a, state: state, guildList: widget.NewItemList(nil)}

	mv.CreateGroup(2, "Games")

	layout, ok := state.GuildLayout(42)
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
	state := &uistate.State{Accounts: &uistate.Accounts{List: []uistate.Account{{ID: 42}}}}
	state.ToggleCollapsedFolder(10)
	a := app.New(discord.WrapSession(session.New("")), data, tui.New())
	mv := &MainView{app: a, state: state, guildList: widget.NewItemList(nil)}
	mv.rebuildGuilds()

	mv.dropGuild(1, 0)

	layout, ok := state.GuildLayout(42)
	if !ok || len(layout) != 1 || !reflect.DeepEqual(layout[0].GuildIDs, []uint64{1, 2}) {
		t.Fatalf("saved layout = %+v,%v", layout, ok)
	}
	if state.IsFolderCollapsed(10) {
		t.Fatal("target group stayed collapsed")
	}
	if len(mv.guildRows) != 3 || mv.guildRows[2].GuildID != 2 {
		t.Fatalf("rendered rows = %+v", mv.guildRows)
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
