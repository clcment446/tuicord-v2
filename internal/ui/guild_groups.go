package ui

import (
	"strings"

	"awesomeProject/internal/app"
	"awesomeProject/internal/store"
	"awesomeProject/internal/uistate"
)

func cleanGroups(groups []store.GuildFolder, guilds []store.Guild) []store.GuildFolder {
	valid := make(map[store.GuildID]bool, len(guilds))
	for _, guild := range guilds {
		if guild.ID != app.DirectMessagesGuildID {
			valid[guild.ID] = true
		}
	}

	seen := make(map[store.GuildID]bool, len(guilds))
	out := make([]store.GuildFolder, 0, len(groups)+len(guilds))
	for _, group := range groups {
		next := store.GuildFolder{ID: group.ID, Name: group.Name, Color: group.Color}
		for _, id := range group.GuildIDs {
			if valid[id] && !seen[id] {
				next.GuildIDs = append(next.GuildIDs, id)
				seen[id] = true
			}
		}
		if len(next.GuildIDs) > 0 {
			out = append(out, next)
		}
	}
	for _, guild := range guilds {
		if guild.ID != app.DirectMessagesGuildID && !seen[guild.ID] {
			out = append(out, store.GuildFolder{GuildIDs: []store.GuildID{guild.ID}})
		}
	}
	return out
}

func moveGuild(groups []store.GuildFolder, guild store.GuildID, target store.GuildRow, after bool) []store.GuildFolder {
	out := copyFolders(groups)
	if guild == 0 || guild == app.DirectMessagesGuildID || target.GuildID == guild {
		return out
	}

	out = pullGuild(out, guild)

	if target.Folder {
		index := folderIndex(out, target.FolderID)
		if index >= 0 {
			out[index].GuildIDs = append(out[index].GuildIDs, guild)
			return out
		}
	}

	if target.GuildID != 0 && target.GuildID != app.DirectMessagesGuildID {
		index := groupIndex(out, target.GuildID)
		if index >= 0 {
			position := guildIndex(out[index].GuildIDs, target.GuildID)
			if after {
				position++
			}
			if len(out[index].GuildIDs) > 1 || out[index].ID != 0 || out[index].Name != "" {
				out[index].GuildIDs = insertGuild(out[index].GuildIDs, position, guild)
				return out
			}
			entry := store.GuildFolder{GuildIDs: []store.GuildID{guild}}
			if after {
				index++
			}
			out = insertFolder(out, index, entry)
			return out
		}
	}

	entry := store.GuildFolder{GuildIDs: []store.GuildID{guild}}
	if target.GuildID == app.DirectMessagesGuildID {
		return insertFolder(out, 0, entry)
	}
	return append(out, entry)
}

func makeGroup(groups []store.GuildFolder, guild store.GuildID, name string) []store.GuildFolder {
	out := copyFolders(groups)
	if guild == 0 || guild == app.DirectMessagesGuildID {
		return out
	}
	entry := store.GuildFolder{ID: nextGroupID(out), Name: name, GuildIDs: []store.GuildID{guild}}
	index := groupIndex(out, guild)
	if index < 0 {
		return append(out, entry)
	}
	out[index].GuildIDs = pullID(out[index].GuildIDs, guild)
	if len(out[index].GuildIDs) == 0 {
		out[index] = entry
		return out
	}
	return insertFolder(out, index+1, entry)
}

func copyFolders(groups []store.GuildFolder) []store.GuildFolder {
	out := make([]store.GuildFolder, len(groups))
	for i, group := range groups {
		out[i] = group
		out[i].GuildIDs = append([]store.GuildID(nil), group.GuildIDs...)
	}
	return out
}

func pullGuild(groups []store.GuildFolder, guild store.GuildID) []store.GuildFolder {
	for i := len(groups) - 1; i >= 0; i-- {
		groups[i].GuildIDs = pullID(groups[i].GuildIDs, guild)
		if len(groups[i].GuildIDs) == 0 {
			groups = append(groups[:i], groups[i+1:]...)
		}
	}
	return groups
}

func pullID(guilds []store.GuildID, guild store.GuildID) []store.GuildID {
	out := guilds[:0]
	for _, id := range guilds {
		if id != guild {
			out = append(out, id)
		}
	}
	return out
}

func groupIndex(groups []store.GuildFolder, guild store.GuildID) int {
	for i, group := range groups {
		if guildIndex(group.GuildIDs, guild) >= 0 {
			return i
		}
	}
	return -1
}

func folderIndex(groups []store.GuildFolder, folder int64) int {
	for i, group := range groups {
		if group.ID == folder && (group.ID != 0 || group.Name != "" || len(group.GuildIDs) > 1) {
			return i
		}
	}
	return -1
}

func guildIndex(guilds []store.GuildID, guild store.GuildID) int {
	for i, id := range guilds {
		if id == guild {
			return i
		}
	}
	return -1
}

func insertGuild(guilds []store.GuildID, index int, guild store.GuildID) []store.GuildID {
	if index < 0 || index > len(guilds) {
		index = len(guilds)
	}
	guilds = append(guilds, 0)
	copy(guilds[index+1:], guilds[index:])
	guilds[index] = guild
	return guilds
}

func insertFolder(groups []store.GuildFolder, index int, group store.GuildFolder) []store.GuildFolder {
	if index < 0 || index > len(groups) {
		index = len(groups)
	}
	groups = append(groups, store.GuildFolder{})
	copy(groups[index+1:], groups[index:])
	groups[index] = group
	return groups
}

func nextGroupID(groups []store.GuildFolder) int64 {
	id := int64(-1)
	for _, group := range groups {
		if group.ID <= id {
			id = group.ID - 1
		}
	}
	return id
}

func (mv *MainView) accountID() uint64 {
	if mv == nil || mv.app == nil {
		return 0
	}
	if id := uint64(mv.app.SelfID()); id != 0 {
		return id
	}
	accounts := mv.state.AccountList()
	index := mv.state.ActiveAccount()
	if index >= 0 && index < len(accounts) {
		return accounts[index].ID
	}
	return 0
}

func (mv *MainView) currentGroups() []store.GuildFolder {
	if mv == nil || mv.app == nil {
		return nil
	}
	data := mv.app.Store()
	groups := data.GuildFolders()
	if saved, ok := mv.state.GuildLayout(mv.accountID()); ok {
		groups = make([]store.GuildFolder, len(saved))
		for i, group := range saved {
			groups[i] = store.GuildFolder{ID: group.ID, Name: group.Name, Color: group.Color}
			for _, id := range group.GuildIDs {
				groups[i].GuildIDs = append(groups[i].GuildIDs, store.GuildID(id))
			}
		}
	}
	return cleanGroups(groups, data.Guilds())
}

func (mv *MainView) saveGroups(groups []store.GuildFolder) {
	saved := make([]uistate.GuildGroup, len(groups))
	for i, group := range groups {
		saved[i] = uistate.GuildGroup{ID: group.ID, Name: group.Name, Color: group.Color}
		for _, id := range group.GuildIDs {
			saved[i].GuildIDs = append(saved[i].GuildIDs, uint64(id))
		}
	}
	mv.state.SetGuildLayout(mv.accountID(), saved)
}

func (mv *MainView) canDragGuild(index int) bool {
	if mv == nil || index < 0 || index >= len(mv.guildRows) {
		return false
	}
	row := mv.guildRows[index]
	return !row.Folder && row.GuildID != 0 && row.GuildID != app.DirectMessagesGuildID
}

func (mv *MainView) dragGuild(from, to int) int {
	if mv == nil || from < 0 || from >= len(mv.guildRows) || to < 0 || to >= len(mv.guildRows) {
		return to
	}
	target := mv.guildRows[to]
	if target.GuildID == app.DirectMessagesGuildID {
		return 1
	}
	if !target.Folder {
		return to
	}
	end := to + 1
	for end < len(mv.guildRows) {
		row := mv.guildRows[end]
		if row.Folder || !row.Indent || row.FolderID != target.FolderID {
			break
		}
		end++
	}
	if from < end {
		end--
	}
	return end
}

func (mv *MainView) dropGuild(from, to int) {
	if mv == nil || from < 0 || from >= len(mv.guildRows) || to < 0 || to >= len(mv.guildRows) {
		return
	}
	source := mv.guildRows[from]
	target := mv.guildRows[to]
	groups := moveGuild(mv.currentGroups(), source.GuildID, target, to > from)
	if source.Pinned {
		mv.state.TogglePinnedGuild(uint64(source.GuildID))
	}
	if target.Folder && target.Collapsed {
		mv.state.ToggleCollapsedFolder(target.FolderID)
	}
	mv.saveGroups(groups)
	mv.persist()
	mv.rebuildGuilds()
	if index := mv.guildRowIndex(source.GuildID); index >= 0 {
		mv.guildList.SetSelectedSilent(index)
	}
}

func (mv *MainView) CreateGroup(guild store.GuildID, name string) {
	if mv == nil || guild == 0 || guild == app.DirectMessagesGuildID {
		return
	}
	if mv.state.IsPinnedGuild(uint64(guild)) {
		mv.state.TogglePinnedGuild(uint64(guild))
	}
	groups := makeGroup(mv.currentGroups(), guild, strings.TrimSpace(name))
	mv.saveGroups(groups)
	mv.persist()
	mv.rebuildGuilds()
	if index := mv.guildRowIndex(guild); index >= 0 {
		mv.guildList.SetSelectedSilent(index)
	}
}
