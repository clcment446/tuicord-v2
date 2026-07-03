package app

import (
	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

// convertChannelKind maps an arikawa channel type to a store.ChannelKind.
func convertChannelKind(t discord.ChannelType) store.ChannelKind {
	switch t {
	case discord.GuildVoice:
		return store.ChannelVoice
	case discord.GuildCategory:
		return store.ChannelCategory
	case discord.DirectMessage, discord.GroupDM:
		return store.ChannelDM
	default:
		return store.ChannelText
	}
}

// convertChannel maps an arikawa channel into a store.Channel.
func convertChannel(c discord.Channel) store.Channel {
	return store.Channel{
		ID:       store.ChannelID(c.ID),
		GuildID:  store.GuildID(c.GuildID),
		Name:     c.Name,
		Kind:     convertChannelKind(c.Type),
		Position: c.Position,
	}
}

// convertMessage maps an arikawa message into a store.Message.
func convertMessage(m discord.Message) store.Message {
	return store.Message{
		ID:        store.MessageID(m.ID),
		ChannelID: store.ChannelID(m.ChannelID),
		Author:    m.Author.DisplayOrUsername(),
		Content:   m.Content,
		Timestamp: m.Timestamp.Time(),
		Nonce:     m.Nonce,
	}
}

// convertMember maps an arikawa member into a store.Member.
func convertMember(m discord.Member) store.Member {
	name := m.Nick
	if name == "" {
		name = m.User.DisplayOrUsername()
	}
	return store.Member{
		ID:   store.UserID(m.User.ID),
		Name: name,
	}
}

// ingestGuild writes a guild and its channels/members into the store.
func ingestGuild(s *store.Store, g *gateway.GuildCreateEvent) {
	s.UpsertGuild(store.Guild{ID: store.GuildID(g.ID), Name: g.Name})
	for _, c := range g.Channels {
		c.GuildID = g.ID
		s.UpsertChannel(convertChannel(c))
	}
	for _, m := range g.Members {
		s.UpsertMember(store.GuildID(g.ID), convertMember(m))
	}
}
