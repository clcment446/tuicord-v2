package store

// This file holds the pure sidebar-ordering functions. They take the store's
// data plus the client-side view preferences (pins, collapsed sets) and return
// a flat list of render rows. Keeping them pure — no *Store receiver, no IO —
// makes the folder/category/pin layout table-testable, which is where the
// fiddly edge cases (unknown ids, collapsed folders, pin de-duplication) live.

// GuildRow is one rendered line in the guild rail. A row is either a folder
// header (Folder true, GuildID zero) or a guild (Folder false). Guilds nested
// under an expanded folder carry Indent; pinned guilds are surfaced in a
// section at the top with Pinned set and are not repeated inside their folder.
type GuildRow struct {
	Folder    bool
	Name      string
	Color     uint32
	Collapsed bool
	Indent    bool
	Pinned    bool
	// GuildID identifies a guild row; zero for folder headers.
	GuildID GuildID
	// FolderID is the folder's own id on header rows, or a guild's parent
	// folder id (zero when the guild is not in a real folder).
	FolderID int64
}

// OrderGuilds flattens folders + guilds into render rows given the client's
// pins and collapsed folders.
//
// Ordering: pinned guilds first (in pins order), then folders/guilds in folder
// order. A folder with an empty Name and a single guild is Discord's encoding
// of an un-foldered guild and renders as a bare top-level guild. Real folders
// render a header row; when not collapsed their guilds follow, indented. Guilds
// present in the store but absent from every folder are appended in base order.
// Pinned guilds are never duplicated inside a folder. Unknown guild ids in a
// folder are skipped.
func OrderGuilds(folders []GuildFolder, guilds []Guild, pinned []GuildID, collapsed map[int64]bool) []GuildRow {
	byID := make(map[GuildID]Guild, len(guilds))
	for _, g := range guilds {
		byID[g.ID] = g
	}

	pinnedSet := make(map[GuildID]bool, len(pinned))
	rows := make([]GuildRow, 0, len(guilds)+len(folders))
	for _, id := range pinned {
		if pinnedSet[id] {
			continue
		}
		if g, ok := byID[id]; ok {
			pinnedSet[id] = true
			rows = append(rows, GuildRow{Name: g.Name, GuildID: g.ID, Pinned: true})
		}
	}

	emitted := make(map[GuildID]bool, len(guilds))
	appendGuild := func(g Guild, indent bool, folderID int64) {
		if pinnedSet[g.ID] || emitted[g.ID] {
			return
		}
		emitted[g.ID] = true
		rows = append(rows, GuildRow{Name: g.Name, GuildID: g.ID, Indent: indent, FolderID: folderID})
	}

	for _, f := range folders {
		if bareFolder(f) {
			if g, ok := byID[f.GuildIDs[0]]; ok {
				appendGuild(g, false, 0)
			}
			continue
		}
		isCollapsed := collapsed[f.ID]
		rows = append(rows, GuildRow{Folder: true, Name: folderName(f), Color: f.Color, Collapsed: isCollapsed, FolderID: f.ID})
		if isCollapsed {
			// Still mark member guilds as emitted so the fallback pass below
			// does not surface them again outside their (collapsed) folder.
			for _, id := range f.GuildIDs {
				if _, ok := byID[id]; ok {
					emitted[id] = true
				}
			}
			continue
		}
		for _, id := range f.GuildIDs {
			if g, ok := byID[id]; ok {
				appendGuild(g, true, f.ID)
			}
		}
	}

	// Guilds not covered by any folder keep their first-seen order at the end.
	for _, g := range guilds {
		appendGuild(g, false, 0)
	}
	return rows
}

func bareFolder(f GuildFolder) bool {
	return f.Name == "" && f.ID == 0 && len(f.GuildIDs) == 1
}

func folderName(f GuildFolder) string {
	if f.Name != "" {
		return f.Name
	}
	return "Folder"
}

// ChannelRow is one rendered line in the channel sidebar: a category header
// (Category true, ChannelID zero) or a channel. Channels under a category carry
// Indent; pinned channels are surfaced at the top with Pinned set and are not
// repeated inside their category. Navigable reports whether selecting the row
// should switch channels (text and DM channels); category headers and voice
// channels are not navigable.
type ChannelRow struct {
	Category  bool
	Name      string
	Kind      ChannelKind
	Collapsed bool
	Indent    bool
	Pinned    bool
	ChannelID ChannelID
}

// Navigable reports whether selecting this row switches to a channel.
func (r ChannelRow) Navigable() bool {
	return !r.Category && (r.Kind == ChannelText || r.Kind == ChannelDM)
}

// GroupChannels flattens a guild's channels into render rows: a pinned section,
// then uncategorized channels, then each category header followed (when not
// collapsed) by its channels.
//
// channels must be pre-sorted (Store.Channels returns Position order). Pinned
// channels appear once, at the top, in the order they occur in channels. A
// category whose id is in collapsed hides its children. Channels whose ParentID
// points at an unknown category are treated as uncategorized. Category headers
// with no surviving children are omitted.
func GroupChannels(channels []Channel, pinned []ChannelID, collapsed map[ChannelID]bool) []ChannelRow {
	pinnedSet := make(map[ChannelID]bool, len(pinned))
	for _, id := range pinned {
		pinnedSet[id] = true
	}

	categories := make(map[ChannelID]bool)
	for _, c := range channels {
		if c.Kind == ChannelCategory {
			categories[c.ID] = true
		}
	}

	rows := make([]ChannelRow, 0, len(channels))
	// Pinned section, in Position order, only content channels.
	for _, c := range channels {
		if c.Kind == ChannelCategory || !pinnedSet[c.ID] {
			continue
		}
		rows = append(rows, channelContentRow(c, false, true))
	}

	// Uncategorized content channels (no parent, or an unknown parent).
	for _, c := range channels {
		if c.Kind == ChannelCategory || pinnedSet[c.ID] {
			continue
		}
		if c.ParentID != 0 && categories[c.ParentID] {
			continue
		}
		rows = append(rows, channelContentRow(c, false, false))
	}

	// Categories with their children.
	for _, cat := range channels {
		if cat.Kind != ChannelCategory {
			continue
		}
		isCollapsed := collapsed[cat.ID]
		children := make([]ChannelRow, 0)
		if !isCollapsed {
			for _, c := range channels {
				if c.Kind == ChannelCategory || pinnedSet[c.ID] || c.ParentID != cat.ID {
					continue
				}
				children = append(children, channelContentRow(c, true, false))
			}
		}
		if !isCollapsed && len(children) == 0 {
			// Empty (or fully-pinned) category: skip its header entirely.
			continue
		}
		rows = append(rows, ChannelRow{Category: true, Name: cat.Name, Kind: ChannelCategory, Collapsed: isCollapsed, ChannelID: cat.ID})
		rows = append(rows, children...)
	}
	return rows
}

func channelContentRow(c Channel, indent, pinned bool) ChannelRow {
	return ChannelRow{Name: c.Name, Kind: c.Kind, Indent: indent, Pinned: pinned, ChannelID: c.ID}
}
