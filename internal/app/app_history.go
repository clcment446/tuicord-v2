// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/discord"
)

func (a *App) directoryRequestSnapshot() directoryRequestSnapshot {
	a.resourceMu.Lock()
	version := a.guildsGate.version
	a.resourceMu.Unlock()
	return directoryRequestSnapshot{
		guilds:      a.store.GuildGenerations(),
		channels:    a.store.ChannelGenerations(),
		gateVersion: version,
	}
}

func (a *App) historyRequestSnapshot(channel store.ChannelID) historyRequestSnapshot {
	a.resourceMu.Lock()
	a.ensureResourceMaps()
	version := a.historyGate.version[channel]
	a.resourceMu.Unlock()
	return historyRequestSnapshot{
		revision:          a.store.Revision(),
		channelGeneration: a.store.ChannelGeneration(channel),
		gateVersion:       version,
	}
}

// LoadHistory fetches recent messages for channel and replaces the local
// history. The REST API returns latest-first; the store keeps oldest-first.
func (a *App) LoadHistory(channel store.ChannelID, limit uint) {
	if a == nil || a.history == nil || channel == 0 {
		return
	}
	version, ok := a.beginHistoryLoad(channel)
	if !ok {
		return
	}
	// Snapshot on the UI goroutine before starting REST. Message revisions,
	// delete tombstones, and the channel lifetime protect gateway mutations that
	// happen while the request is in flight.
	snapshot := a.historyRequestSnapshot(channel)
	snapshot.gateVersion = version
	go a.loadHistoryFrom(channel, limit, snapshot)
}

// loadHistory is the synchronous test seam. Production callers use
// LoadHistory, which captures the same snapshot before starting its goroutine.
func (a *App) loadHistory(channel store.ChannelID, limit uint) {
	a.loadHistoryFrom(channel, limit, a.historyRequestSnapshot(channel))
}

func (a *App) loadHistoryFrom(channel store.ChannelID, limit uint, snapshot historyRequestSnapshot) {
	messages, err := a.history.Messages(discord.ChannelID(channel), limit)
	if err != nil {
		a.ui.Post(func() {
			if a.store.ChannelGeneration(channel) != snapshot.channelGeneration || !a.historyLoadCurrent(channel, snapshot.gateVersion) {
				return
			}
			a.finishHistoryLoad(channel, false)
			if a.onError != nil {
				a.onError(err)
			}
		})
		return
	}
	converted := make([]store.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		converted = append(converted, convertMessage(messages[i]))
	}
	a.ui.Post(func() {
		if a.store.ChannelGeneration(channel) != snapshot.channelGeneration || !a.historyLoadCurrent(channel, snapshot.gateVersion) {
			// The request belongs to a deleted or replaced channel lifetime. Its
			// gate was invalidated by deletion; do not touch the new lifetime's gate.
			return
		}
		if a.store.TombstonesPrunedSince(channel, snapshot.revision) {
			// More in-flight deletes arrived than the bounded tombstone cache can
			// identify. Discard the stale page and allow a fresh request to retry.
			a.finishHistoryLoad(channel, false)
			return
		}
		if ch, ok := a.store.Channel(channel); ok && ch.GuildID != 0 && ch.GuildID != DirectMessagesGuildID {
			guild := discord.GuildID(ch.GuildID)
			for _, message := range messages {
				a.store.RememberMemberIdentity(ch.GuildID, convertMember(discord.Member{User: message.Author}, guild))
			}
		}
		current := a.store.Messages(channel)
		a.store.SetMessages(channel, mergeInitialHistory(a.store, channel, converted, current, snapshot.revision))
		a.finishHistoryLoad(channel, true)
		if limit == 0 || len(messages) < int(limit) {
			a.markHistoryExhausted(channel)
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// mergeInitialHistory installs the REST page while retaining newer gateway
// versions and arrivals. Delete tombstones win over every REST copy.
// SetMessages separately preserves pending/failed local echoes.
func mergeInitialHistory(st *store.Store, channel store.ChannelID, incoming, current []store.Message, requestRevision uint64) []store.Message {
	currentByID := make(map[store.MessageID]store.Message, len(current))
	for _, message := range current {
		if message.ID != 0 {
			currentByID[message.ID] = message
		}
	}
	known := make(map[store.MessageID]struct{}, len(incoming)+len(current))
	merged := make([]store.Message, 0, len(incoming)+len(current))
	for _, message := range incoming {
		if message.ID != 0 {
			if st.MessageTombstoned(channel, message.ID) {
				continue
			}
			if _, duplicate := known[message.ID]; duplicate {
				continue
			}
			known[message.ID] = struct{}{}
			if live, ok := currentByID[message.ID]; ok && live.Rev() > requestRevision {
				message = live
			}
		}
		merged = append(merged, message)
	}
	for _, message := range current {
		if message.ID == 0 || message.Pending || message.Failed || message.Rev() <= requestRevision {
			continue
		}
		if st.MessageTombstoned(channel, message.ID) {
			continue
		}
		if _, inRESTPage := known[message.ID]; inRESTPage {
			continue
		}
		known[message.ID] = struct{}{}
		merged = append(merged, message)
	}
	return merged
}

// LoadOlderHistory fetches the next page before the oldest cached message.
// Calls made while a page is in flight or when Discord has no older messages
// are ignored.
func (a *App) LoadOlderHistory(channel store.ChannelID) {
	if a == nil || a.history == nil || channel == 0 {
		return
	}
	version, ok := a.beginOlderHistoryLoad(channel)
	if !ok {
		return
	}
	messages := a.store.Messages(channel)
	if len(messages) == 0 {
		a.finishOlderHistory(channel, true)
		return
	}
	before := messages[0].ID
	if before == 0 {
		a.finishOlderHistory(channel, true)
		return
	}
	snapshot := a.historyRequestSnapshot(channel)
	snapshot.gateVersion = version
	go a.loadOlderHistoryFrom(channel, discord.MessageID(before), 50, snapshot)
}

// loadOlderHistory is the synchronous test seam.
func (a *App) loadOlderHistory(channel store.ChannelID, before discord.MessageID, limit uint) {
	a.loadOlderHistoryFrom(channel, before, limit, a.historyRequestSnapshot(channel))
}

func (a *App) loadOlderHistoryFrom(channel store.ChannelID, before discord.MessageID, limit uint, snapshot historyRequestSnapshot) {
	messages, err := a.history.MessagesBefore(discord.ChannelID(channel), before, limit)
	if err != nil {
		a.ui.Post(func() {
			if a.store.ChannelGeneration(channel) != snapshot.channelGeneration || !a.historyLoadCurrent(channel, snapshot.gateVersion) {
				return
			}
			a.finishOlderHistory(channel, false)
			a.reportError(err)
		})
		return
	}
	converted := make([]store.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		converted = append(converted, convertMessage(messages[i]))
	}
	a.ui.Post(func() {
		if a.store.ChannelGeneration(channel) != snapshot.channelGeneration || !a.historyLoadCurrent(channel, snapshot.gateVersion) {
			return
		}
		if a.store.TombstonesPrunedSince(channel, snapshot.revision) {
			a.finishOlderHistory(channel, false)
			return
		}
		if ch, ok := a.store.Channel(channel); ok && ch.GuildID != 0 && ch.GuildID != DirectMessagesGuildID {
			guild := discord.GuildID(ch.GuildID)
			for _, message := range messages {
				a.store.RememberMemberIdentity(ch.GuildID, convertMember(discord.Member{User: message.Author}, guild))
			}
		}
		a.store.PrependMessagesSince(channel, converted, snapshot.revision)
		a.finishOlderHistory(channel, len(messages) < int(limit))
		if len(converted) > 0 && a.onChange != nil {
			a.onChange()
		}
	})
}
