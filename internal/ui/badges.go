package ui

import "awesomeProject/internal/store"

const serverUnreadDot = "●"

type serverUnreadStatus uint8

const (
	serverRead serverUnreadStatus = iota
	serverUnread
	serverMentioned
)

type serverBadgeKind uint8

const (
	noServerBadge serverBadgeKind = iota
	serverUnreadBadgeKind
	serverMentionBadge
)

func serverUnreadBadge(status serverUnreadStatus) (string, serverBadgeKind) {
	switch status {
	case serverMentioned:
		return serverUnreadDot, serverMentionBadge
	case serverUnread:
		return serverUnreadDot, serverUnreadBadgeKind
	default:
		return "", noServerBadge
	}
}

// This file holds the pure channel-badge selection used by the sidebar. It maps
// a channel's kind (and its rules-channel status) to the small glyph shown
// before the name. Keeping it pure — kind + rules flag in, string out — makes
// the glyph table and the ASCII fallback table straightforward to unit-test.

// channelGlyphs pairs the Unicode badge with its ASCII fallback for one channel
// role. Every glyph is single terminal cell wide so the sidebar stays aligned
// and clean on an 80×24 terminal and under NO_COLOR.
type channelGlyphs struct {
	unicode string
	ascii   string
}

var (
	badgeText         = channelGlyphs{"#", "#"}
	badgeVoice        = channelGlyphs{"~", "~"}
	badgeAnnouncement = channelGlyphs{"!", "!"}
	badgeForum        = channelGlyphs{"☰", "="}
	badgeThread       = channelGlyphs{"⤷", ">"}
	badgeRules        = channelGlyphs{"§", "R"}
)

// channelBadge returns the sidebar glyph for a channel: rules channels win over
// their underlying kind, then announcement/forum/thread/voice, defaulting to the
// text hash. DM channels carry no badge (they are titled by recipient). When
// ascii is true the ASCII fallback glyph is returned.
//
// The rules match is passed in (rather than derived) so the function stays pure
// and does not need the whole guild.
func channelBadge(kind store.ChannelKind, isRules, ascii bool) string {
	g := badgeFor(kind, isRules)
	if ascii {
		return g.ascii
	}
	return g.unicode
}

func badgeFor(kind store.ChannelKind, isRules bool) channelGlyphs {
	if isRules {
		return badgeRules
	}
	switch kind {
	case store.ChannelVoice:
		return badgeVoice
	case store.ChannelAnnouncement:
		return badgeAnnouncement
	case store.ChannelForum:
		return badgeForum
	case store.ChannelThread:
		return badgeThread
	default:
		return badgeText
	}
}

// channelPrefixBadge builds the leading "<glyph> " string for a channel row, or
// "" for DM channels which display no badge.
func channelPrefixBadge(kind store.ChannelKind, isRules, ascii bool) string {
	if kind == store.ChannelDM {
		return ""
	}
	return channelBadge(kind, isRules, ascii) + " "
}
