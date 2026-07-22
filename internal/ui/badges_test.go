package ui

import (
	"testing"

	"awesomeProject/internal/store"
)

func TestChannelBadgeUnicode(t *testing.T) {
	cases := []struct {
		kind    store.ChannelKind
		isRules bool
		want    string
	}{
		{store.ChannelText, false, "#"},
		{store.ChannelVoice, false, "~"},
		{store.ChannelAnnouncement, false, "!"},
		{store.ChannelForum, false, "☰"},
		{store.ChannelThread, false, "⤷"},
		{store.ChannelText, true, "§"}, // rules wins over the underlying kind
	}
	for _, c := range cases {
		if got := channelBadge(c.kind, c.isRules, false); got != c.want {
			t.Errorf("channelBadge(%v, rules=%v) = %q, want %q", c.kind, c.isRules, got, c.want)
		}
	}
}

func TestChannelBadgeASCIIFallback(t *testing.T) {
	cases := []struct {
		kind    store.ChannelKind
		isRules bool
		want    string
	}{
		{store.ChannelForum, false, "="},
		{store.ChannelThread, false, ">"},
		{store.ChannelAnnouncement, false, "!"},
		{store.ChannelText, true, "R"},
	}
	for _, c := range cases {
		got := channelBadge(c.kind, c.isRules, true)
		if got != c.want {
			t.Errorf("channelBadge ascii(%v, rules=%v) = %q, want %q", c.kind, c.isRules, got, c.want)
		}
		// ASCII glyphs must be single-byte, single-cell so the sidebar stays
		// aligned under NO_COLOR.
		if len(got) != 1 {
			t.Errorf("ascii badge %q is not a single byte", got)
		}
	}
}

func TestChannelPrefixBadgeDMEmpty(t *testing.T) {
	if got := channelPrefixBadge(store.ChannelDM, false, false); got != "" {
		t.Errorf("DM prefix = %q, want empty", got)
	}
	if got := channelPrefixBadge(store.ChannelText, false, false); got != "# " {
		t.Errorf("text prefix = %q, want %q", got, "# ")
	}
}

func TestServerUnreadBadgeUsesMentionPrecedence(t *testing.T) {
	cases := []struct {
		status serverUnreadStatus
		want   string
	}{
		{serverRead, ""},
		{serverUnread, serverUnreadDot},
		{serverMentioned, serverUnreadDot},
	}
	for _, tc := range cases {
		badge, kind := serverUnreadBadge(tc.status)
		if badge != tc.want {
			t.Errorf("serverUnreadBadge(%v) = %q, want %q", tc.status, badge, tc.want)
		}
		if tc.status == serverMentioned && kind != serverMentionBadge {
			t.Errorf("mention status kind = %v, want mention", kind)
		}
	}
}
