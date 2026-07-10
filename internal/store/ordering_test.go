package store

import "testing"

func guildRowNames(rows []GuildRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case r.Folder:
			out[i] = "[" + r.Name + "]"
		case r.Pinned:
			out[i] = "*" + r.Name
		case r.Indent:
			out[i] = "  " + r.Name
		default:
			out[i] = r.Name
		}
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOrderGuildsNoFolders(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	rows := OrderGuilds(nil, guilds, nil, nil)
	got := guildRowNames(rows)
	want := []string{"A", "B", "C"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestOrderGuildsPinnedFirstNoDuplicate(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	rows := OrderGuilds(nil, guilds, []GuildID{3, 1}, nil)
	got := guildRowNames(rows)
	want := []string{"*C", "*A", "B"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestOrderGuildsBareFolderIsTopLevel(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	folders := []GuildFolder{
		{ID: 0, Name: "", GuildIDs: []GuildID{1}},
		{ID: 0, Name: "", GuildIDs: []GuildID{2}},
	}
	rows := OrderGuilds(folders, guilds, nil, nil)
	got := guildRowNames(rows)
	want := []string{"A", "B"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestOrderGuildsRealFolderExpanded(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	folders := []GuildFolder{
		{ID: 10, Name: "Work", GuildIDs: []GuildID{1, 2}},
		{ID: 0, Name: "", GuildIDs: []GuildID{3}},
	}
	rows := OrderGuilds(folders, guilds, nil, nil)
	got := guildRowNames(rows)
	want := []string{"[Work]", "  A", "  B", "C"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if rows[0].FolderID != 10 || rows[1].FolderID != 10 {
		t.Fatalf("folder ids not propagated: %+v", rows[:2])
	}
}

func TestOrderGuildsCollapsedFolderHidesChildren(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}
	folders := []GuildFolder{
		{ID: 10, Name: "Work", GuildIDs: []GuildID{1, 2}},
		{ID: 0, Name: "", GuildIDs: []GuildID{3}},
	}
	rows := OrderGuilds(folders, guilds, nil, map[int64]bool{10: true})
	got := guildRowNames(rows)
	want := []string{"[Work]", "C"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if !rows[0].Collapsed {
		t.Fatalf("folder header should be collapsed")
	}
}

func TestOrderGuildsPinnedNotRepeatedInFolder(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	folders := []GuildFolder{{ID: 10, Name: "Work", GuildIDs: []GuildID{1, 2}}}
	rows := OrderGuilds(folders, guilds, []GuildID{1}, nil)
	got := guildRowNames(rows)
	want := []string{"*A", "[Work]", "  B"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestOrderGuildsUnknownFolderMemberSkipped(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}}
	folders := []GuildFolder{{ID: 10, Name: "Work", GuildIDs: []GuildID{1, 99}}}
	rows := OrderGuilds(folders, guilds, nil, nil)
	got := guildRowNames(rows)
	want := []string{"[Work]", "  A"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestOrderGuildsUnnamedRealFolderFallbackName(t *testing.T) {
	guilds := []Guild{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	// A real folder (id set, two guilds) with no name renders a fallback label.
	folders := []GuildFolder{{ID: 5, Name: "", GuildIDs: []GuildID{1, 2}}}
	rows := OrderGuilds(folders, guilds, nil, nil)
	if !rows[0].Folder || rows[0].Name != "Folder" {
		t.Fatalf("row[0] = %+v, want folder header named 'Folder'", rows[0])
	}
}

func channelRowNames(rows []ChannelRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case r.Category:
			out[i] = "[" + r.Name + "]"
		case r.Pinned:
			out[i] = "*" + r.Name
		case r.Indent:
			out[i] = "  " + r.Name
		default:
			out[i] = r.Name
		}
	}
	return out
}

func TestGroupChannelsCategoriesAndUncategorized(t *testing.T) {
	channels := []Channel{
		{ID: 1, Name: "welcome", Kind: ChannelText, Position: 0},
		{ID: 10, Name: "TEXT", Kind: ChannelCategory, Position: 1},
		{ID: 11, Name: "general", Kind: ChannelText, Position: 2, ParentID: 10},
		{ID: 12, Name: "dev", Kind: ChannelText, Position: 3, ParentID: 10},
		{ID: 20, Name: "VOICE", Kind: ChannelCategory, Position: 4},
		{ID: 21, Name: "lounge", Kind: ChannelVoice, Position: 5, ParentID: 20},
	}
	rows := GroupChannels(channels, nil, nil)
	got := channelRowNames(rows)
	want := []string{"welcome", "[TEXT]", "  general", "  dev", "[VOICE]", "  lounge"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestGroupChannelsCollapsedCategory(t *testing.T) {
	channels := []Channel{
		{ID: 10, Name: "TEXT", Kind: ChannelCategory, Position: 0},
		{ID: 11, Name: "general", Kind: ChannelText, Position: 1, ParentID: 10},
	}
	rows := GroupChannels(channels, nil, map[ChannelID]bool{10: true})
	got := channelRowNames(rows)
	want := []string{"[TEXT]"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if !rows[0].Collapsed {
		t.Fatalf("category should be collapsed")
	}
}

func TestGroupChannelsPinnedSectionNoDuplicate(t *testing.T) {
	channels := []Channel{
		{ID: 10, Name: "TEXT", Kind: ChannelCategory, Position: 0},
		{ID: 11, Name: "general", Kind: ChannelText, Position: 1, ParentID: 10},
		{ID: 12, Name: "dev", Kind: ChannelText, Position: 2, ParentID: 10},
	}
	rows := GroupChannels(channels, []ChannelID{12}, nil)
	got := channelRowNames(rows)
	want := []string{"*dev", "[TEXT]", "  general"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestGroupChannelsEmptyCategoryHidden(t *testing.T) {
	channels := []Channel{
		{ID: 10, Name: "EMPTY", Kind: ChannelCategory, Position: 0},
		{ID: 20, Name: "TEXT", Kind: ChannelCategory, Position: 1},
		{ID: 21, Name: "general", Kind: ChannelText, Position: 2, ParentID: 20},
	}
	rows := GroupChannels(channels, nil, nil)
	got := channelRowNames(rows)
	want := []string{"[TEXT]", "  general"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestGroupChannelsUnknownParentIsUncategorized(t *testing.T) {
	channels := []Channel{
		{ID: 11, Name: "orphan", Kind: ChannelText, Position: 0, ParentID: 999},
	}
	rows := GroupChannels(channels, nil, nil)
	got := channelRowNames(rows)
	want := []string{"orphan"}
	if !eqStrings(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestChannelRowNavigable(t *testing.T) {
	cases := []struct {
		row  ChannelRow
		want bool
	}{
		{ChannelRow{Kind: ChannelText}, true},
		{ChannelRow{Kind: ChannelDM}, true},
		{ChannelRow{Kind: ChannelVoice}, false},
		{ChannelRow{Category: true, Kind: ChannelText}, false},
	}
	for _, c := range cases {
		if got := c.row.Navigable(); got != c.want {
			t.Fatalf("Navigable(%+v) = %v, want %v", c.row, got, c.want)
		}
	}
}
