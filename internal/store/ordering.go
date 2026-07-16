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
	// Depth is the indentation level: 0 top-level, 1 under a category (or a
	// thread under a top-level channel), 2 a thread under a categorized channel.
	// Indent stays true whenever Depth > 0 for backwards compatibility.
	Depth int
	// Thread marks a thread sub-item (a channel nested under its parent
	// channel). Thread rows are navigable and open as the active channel.
	Thread bool
}

// Navigable reports whether selecting this row switches the main view to a
// channel. Category headers and voice channels are not navigable; forum
// channels are (selecting one opens the post-list view rather than a chat).
func (r ChannelRow) Navigable() bool {
	if r.Category {
		return false
	}
	switch r.Kind {
	case ChannelText, ChannelDM, ChannelAnnouncement, ChannelThread, ChannelForum:
		return true
	default:
		return false
	}
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
	return GroupChannelsWithPriority(channels, pinned, collapsed, nil)
}

// GroupChannelsWithPriority promotes local favorites first, then unread direct
// messages with pings, then other pinged channels and threads. Promoted rows
// are removed from their ordinary parent/category position so they never
// appear twice. The remaining rows preserve Discord's position/activity order.
func GroupChannelsWithPriority(channels []Channel, pinned []ChannelID, collapsed map[ChannelID]bool, priority map[ChannelID]bool) []ChannelRow {
	pinnedSet := make(map[ChannelID]bool, len(pinned))
	for _, id := range pinned {
		pinnedSet[id] = true
	}

	byID := make(map[ChannelID]Channel, len(channels))
	categories := make(map[ChannelID]bool)
	for _, c := range channels {
		byID[c.ID] = c
		if c.Kind == ChannelCategory {
			categories[c.ID] = true
		}
	}

	// Active threads grouped under their parent, but only when the parent is a
	// text or announcement channel — forum posts live in the forum's post view,
	// not the sidebar.
	threadsByParent := make(map[ChannelID][]Channel)
	for _, c := range channels {
		if c.Kind != ChannelThread || c.Thread == nil || c.Thread.Archived {
			continue
		}
		parent, ok := byID[c.ParentID]
		if !ok || (parent.Kind != ChannelText && parent.Kind != ChannelAnnouncement) {
			continue
		}
		threadsByParent[c.ParentID] = append(threadsByParent[c.ParentID], c)
	}
	for _, ts := range threadsByParent {
		sortThreads(ts)
	}

	rows := make([]ChannelRow, 0, len(channels))
	promoted := make(map[ChannelID]bool, len(pinned)+len(priority))
	for _, id := range pinned {
		promoted[id] = true
	}
	for id := range priority {
		if !pinnedSet[id] {
			promoted[id] = true
		}
	}
	emitPromoted := func(c Channel, pinnedRow bool) {
		if c.Kind == ChannelThread {
			rows = append(rows, threadRow(c, 0, pinnedRow))
			return
		}
		rows = append(rows, channelContentRow(c, 0, pinnedRow))
	}

	// Favorite rows, in Discord position order.
	for _, c := range channels {
		if c.Kind == ChannelCategory || !pinnedSet[c.ID] {
			continue
		}
		emitPromoted(c, true)
	}
	// Pinged DMs are the first attention tier, immediately after favorites.
	for _, c := range channels {
		if c.Kind == ChannelDM && priority[c.ID] && !pinnedSet[c.ID] {
			emitPromoted(c, false)
		}
	}
	// Other pinged channels and threads follow, still ahead of ordinary rows.
	for _, c := range channels {
		if c.Kind == ChannelCategory || c.Kind == ChannelDM || !priority[c.ID] || pinnedSet[c.ID] {
			continue
		}
		emitPromoted(c, false)
	}
	emit := func(c Channel, depth int) {
		rows = append(rows, channelContentRow(c, depth, false))
		for _, t := range threadsByParent[c.ID] {
			if !promoted[t.ID] {
				rows = append(rows, threadRow(t, depth+1, false))
			}
		}
	}

	// Uncategorized content channels (no parent, or an unknown parent).
	for _, c := range channels {
		if c.Kind == ChannelCategory || c.Kind == ChannelThread || promoted[c.ID] {
			continue
		}
		if c.ParentID != 0 && categories[c.ParentID] {
			continue
		}
		emit(c, 0)
	}

	// Categories with their children.
	for _, cat := range channels {
		if cat.Kind != ChannelCategory {
			continue
		}
		isCollapsed := collapsed[cat.ID]
		var children []ChannelRow
		if !isCollapsed {
			for _, c := range channels {
				if c.Kind == ChannelCategory || c.Kind == ChannelThread || promoted[c.ID] || c.ParentID != cat.ID {
					continue
				}
				children = append(children, channelContentRow(c, 1, false))
				for _, t := range threadsByParent[c.ID] {
					if !promoted[t.ID] {
						children = append(children, threadRow(t, 2, false))
					}
				}
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

func channelContentRow(c Channel, depth int, pinned bool) ChannelRow {
	return ChannelRow{Name: c.Name, Kind: c.Kind, Depth: depth, Indent: depth > 0, Pinned: pinned, ChannelID: c.ID}
}

func threadRow(c Channel, depth int, pinned bool) ChannelRow {
	return ChannelRow{Name: c.Name, Kind: ChannelThread, Depth: depth, Indent: depth > 0, Thread: true, Pinned: pinned, ChannelID: c.ID}
}
