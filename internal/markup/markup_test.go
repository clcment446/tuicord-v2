package markup

import (
	"testing"
	"time"
)

// testResolver returns a Resolver with canned member and channel lookups.
func testResolver() Resolver {
	return Resolver{
		Member: func(id uint64) (string, bool) {
			if id == 42 {
				return "alice", true
			}
			return "", false
		},
		Channel: func(id uint64) (string, bool) {
			if id == 7 {
				return "general", true
			}
			return "", false
		},
	}
}

// testResolverFull returns a Resolver with member, channel, role, guild,
// and a pinned Now so timestamp tests are deterministic.
func testResolverFull() Resolver {
	r := testResolver()
	r.Role = func(id uint64) (string, uint32, bool) {
		switch id {
		case 100:
			return "Moderator", 0xFF5500, true
		case 101:
			return "Member", 0, true // color-less role
		}
		return "", 0, false
	}
	r.Guild = func(id uint64) (string, bool) {
		if id == 999 {
			return "My Server", true
		}
		return "", false
	}
	// Pin now to a known instant so relative tests are deterministic.
	r.Now = func() time.Time {
		return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	}
	return r
}

// ── Existing tests ──────────────────────────────────────────────────────────

func TestParsePlainText(t *testing.T) {
	spans := Parse("hello world", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_Text || spans[0].Text != "hello world" {
		t.Fatalf("spans = %+v", spans)
	}
}

func TestParseBoldItalicCode(t *testing.T) {
	spans := Parse("a **b** c *d* `e`", Resolver{})
	want := []Span{
		{Kind: Kind_Text, Text: "a "},
		{Kind: Kind_Bold, Text: "b"},
		{Kind: Kind_Text, Text: " c "},
		{Kind: Kind_Italic, Text: "d"},
		{Kind: Kind_Text, Text: " "},
		{Kind: Kind_Code, Text: "e"},
	}
	assertSpans(t, spans, want)
}

func TestParseCodeBlockSuppressesInner(t *testing.T) {
	spans := Parse("```**not bold** <@42>```", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_CodeBlock {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Text != "**not bold** <@42>" {
		t.Errorf("code block text = %q", spans[0].Text)
	}
}

func TestParseMentionAndChannel(t *testing.T) {
	spans := Parse("hi <@42> in <#7>", testResolver())
	want := []Span{
		{Kind: Kind_Text, Text: "hi "},
		{Kind: Kind_Mention, Text: "@alice", Action: &Action{Kind: ActionUserMention, Target: "42"}},
		{Kind: Kind_Text, Text: " in "},
		{Kind: Kind_ChannelMention, Text: "#general"},
	}
	assertSpans(t, spans, want)
}

func TestMentionEntitiesCarryClickActions(t *testing.T) {
	spans := Parse("<@42> <@&7>", Resolver{Member: func(uint64) (string, bool) { return "alice", true }, Role: func(uint64) (string, uint32, bool) { return "mod", 0, true }})
	if spans[0].Action == nil || spans[0].Action.Kind != ActionUserMention || spans[0].Action.Target != "42" {
		t.Fatalf("user action = %+v", spans[0].Action)
	}
	if spans[2].Action == nil || spans[2].Action.Kind != ActionRoleMention || spans[2].Action.Target != "7" {
		t.Fatalf("role action = %+v", spans[2].Action)
	}
}

func TestParseNicknameMention(t *testing.T) {
	spans := Parse("<@!42>", testResolver())
	if len(spans) != 1 || spans[0].Kind != Kind_Mention || spans[0].Text != "@alice" {
		t.Fatalf("spans = %+v", spans)
	}
}

func TestParseUnknownMentionDegrades(t *testing.T) {
	spans := Parse("<@999>", Resolver{})
	if len(spans) != 1 || spans[0].Text != "@unknown-user" {
		t.Fatalf("spans = %+v", spans)
	}
}

func TestParseEmoji(t *testing.T) {
	spans := Parse("<:wave:123> <a:spin:456>", Resolver{})
	want := []Span{
		{Kind: Kind_Emoji, Text: ":wave:", EmojiID: 123, EmojiAnimated: false},
		{Kind: Kind_Text, Text: " "},
		{Kind: Kind_Emoji, Text: ":spin:", EmojiID: 456, EmojiAnimated: true},
	}
	assertSpans(t, spans, want)
}

func TestParseLink(t *testing.T) {
	spans := Parse("see [docs](https://example.com)", Resolver{})
	if len(spans) != 2 || spans[1].Kind != Kind_Link {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[1].Text != "docs" || spans[1].URL != "https://example.com" {
		t.Errorf("link span = %+v", spans[1])
	}
}

func TestParseMarkedFakeNitroLinks(t *testing.T) {
	emojiURL := "https://cdn.discordapp.com/emojis/7.gif?size=48&name=spin"
	stickerURL := "https://media.discordapp.net/stickers/42.png"
	spans := Parse("[emoji_spin]("+emojiURL+") [sticker_hello]("+stickerURL+")", Resolver{})
	want := []Span{
		{Kind: Kind_FakeEmoji, Text: "spin", URL: emojiURL},
		{Kind: Kind_Text, Text: " "},
		{Kind: Kind_FakeSticker, Text: "hello", URL: stickerURL},
	}
	assertSpans(t, spans, want)
}

func TestParseMarkerLookingOrdinaryLinks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		label string
		url   string
	}{
		{"empty emoji name", "[emoji_](https://example.com)", "emoji_", "https://example.com"},
		{"emoji label with non-CDN URL", "[emoji_release-notes](https://example.com/docs)", "emoji_release-notes", "https://example.com/docs"},
		{"sticker label with emoji URL", "[sticker_wave](https://cdn.discordapp.com/emojis/7.png)", "sticker_wave", "https://cdn.discordapp.com/emojis/7.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := Parse(tt.input, Resolver{})
			assertSpans(t, spans, []Span{{Kind: Kind_Link, Text: tt.label, URL: tt.url}})
		})
	}
}

func TestParseMalformedDegradesToText(t *testing.T) {
	// Unterminated constructs stay literal.
	for _, in := range []string{"**bold", "`code", "[label](url", "<@notanumber>", "<broken"} {
		spans := Parse(in, Resolver{})
		if len(spans) != 1 || spans[0].Kind != Kind_Text || spans[0].Text != in {
			t.Errorf("Parse(%q) = %+v, want single text span", in, spans)
		}
	}
}

func TestParseExtendedInlineConstructs(t *testing.T) {
	spans := Parse("a __u__ b ~~s~~ c ||sp||", Resolver{})
	want := []Span{
		{Kind: Kind_Text, Text: "a "},
		{Kind: Kind_Underline, Text: "u"},
		{Kind: Kind_Text, Text: " b "},
		{Kind: Kind_Strike, Text: "s"},
		{Kind: Kind_Text, Text: " c "},
		{Kind: Kind_Spoiler, Text: "sp"},
	}
	assertSpans(t, spans, want)
}

func TestParseStackedInlineFormatting(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		format Format
	}{
		{
			name:   "bold underline",
			input:  "**__text__**",
			format: FormatBold | FormatUnderline,
		},
		{
			name:   "underline bold",
			input:  "__**text**__",
			format: FormatBold | FormatUnderline,
		},
		{
			name:   "strike italic",
			input:  "~~*text*~~",
			format: FormatStrike | FormatItalic,
		},
		{
			name:   "spoiler underline strike",
			input:  "||__~~text~~__||",
			format: FormatSpoiler | FormatUnderline | FormatStrike,
		},
		{
			name:   "triple star",
			input:  "***text***",
			format: FormatBold | FormatItalic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := Parse(tt.input, Resolver{})
			want := []Span{{Kind: Kind_Text, Text: "text", Format: tt.format}}
			assertSpans(t, spans, want)
		})
	}
}

func TestParseFormattingAppliesToNestedEntities(t *testing.T) {
	spans := Parse("**hi <@42>**", testResolver())
	want := []Span{
		{Kind: Kind_Bold, Text: "hi "},
		{Kind: Kind_Mention, Text: "@alice", Format: FormatBold, Action: &Action{Kind: ActionUserMention, Target: "42"}},
	}
	assertSpans(t, spans, want)
}

func TestParseHeaderStripsMarker(t *testing.T) {
	spans := Parse("## Title\nbody", Resolver{})
	want := []Span{
		{Kind: Kind_Header, Text: "Title"},
		{Kind: Kind_Text, Text: "\nbody"},
	}
	assertSpans(t, spans, want)
}

func TestParseBlockquoteAddsGutter(t *testing.T) {
	spans := Parse("> hi", Resolver{})
	assertSpans(t, spans, []Span{
		{Kind: Kind_Quote, Text: "▏ "},
		{Kind: Kind_Text, Text: "hi", Quoted: true},
	})

	multi := Parse(">>> one\ntwo", Resolver{})
	assertSpans(t, multi, []Span{
		{Kind: Kind_Quote, Text: "▏ "},
		{Kind: Kind_Text, Text: "one", Quoted: true},
		{Kind: Kind_Quote, Text: "\n▏ "},
		{Kind: Kind_Text, Text: "two", Quoted: true},
	})
}

func TestParseBlockquoteParsesCustomEmoji(t *testing.T) {
	spans := Parse("> <:wave:123>", Resolver{})
	if len(spans) != 2 {
		t.Fatalf("spans = %+v, want gutter and emoji", spans)
	}
	if spans[1].Kind != Kind_Emoji || spans[1].EmojiID != 123 || !spans[1].Quoted {
		t.Fatalf("quoted emoji span = %+v", spans[1])
	}

	spans = Parse(">>> <:wave:123>\n<a:spin:456>", Resolver{})
	if len(spans) != 4 || spans[1].Kind != Kind_Emoji || spans[3].Kind != Kind_Emoji {
		t.Fatalf("multiline quoted emoji spans = %+v", spans)
	}
}

func TestHeaderMarkerOnlyAtLineStart(t *testing.T) {
	spans := Parse("a # b", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_Text || spans[0].Text != "a # b" {
		t.Fatalf("mid-line # should be literal, got %+v", spans)
	}
}

// ── Span field defaults ──────────────────────────────────────────────────────

func TestOrdinarySpansHaveZeroColorAndNoAction(t *testing.T) {
	// Ensure FG, EmojiID, EmojiAnimated, and Action are zero/nil for plain spans.
	spans := Parse("hello **world**", Resolver{})
	for _, s := range spans {
		if s.FG != 0 {
			t.Errorf("FG should be 0 for ordinary span, got %d", s.FG)
		}
		if s.EmojiID != 0 {
			t.Errorf("EmojiID should be 0, got %d", s.EmojiID)
		}
		if s.EmojiAnimated {
			t.Errorf("EmojiAnimated should be false")
		}
		if s.Action != nil {
			t.Errorf("Action should be nil, got %+v", s.Action)
		}
	}
}

// ── Role mentions ────────────────────────────────────────────────────────────

func TestParseRoleMentionResolved(t *testing.T) {
	// Arrange
	res := testResolverFull()
	// Act
	spans := Parse("<@&100>", res)
	// Assert
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %+v", spans)
	}
	s := spans[0]
	if s.Kind != Kind_RoleMention {
		t.Errorf("Kind = %v, want Kind_RoleMention", s.Kind)
	}
	if s.Text != "@Moderator" {
		t.Errorf("Text = %q, want %q", s.Text, "@Moderator")
	}
	if s.FG != 0xFF5500 {
		t.Errorf("FG = %#x, want %#x", s.FG, 0xFF5500)
	}
}

func TestParseRoleMentionColorlessRole(t *testing.T) {
	// Arrange — role 101 exists but has no color
	res := testResolverFull()
	// Act
	spans := Parse("<@&101>", res)
	// Assert
	if len(spans) != 1 || spans[0].Kind != Kind_RoleMention {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].FG != 0 {
		t.Errorf("FG = %#x, want 0 for colorless role", spans[0].FG)
	}
	if spans[0].Text != "@Member" {
		t.Errorf("Text = %q, want @Member", spans[0].Text)
	}
}

func TestParseRoleMentionUnknown(t *testing.T) {
	// Arrange — no Role resolver at all
	spans := Parse("<@&999>", Resolver{})
	// Assert
	if len(spans) != 1 || spans[0].Kind != Kind_RoleMention {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Text != "@unknown-role" {
		t.Errorf("Text = %q, want @unknown-role", spans[0].Text)
	}
	if spans[0].FG != 0 {
		t.Errorf("FG = %#x, want 0 for unknown role", spans[0].FG)
	}
}

func TestParseRoleMentionInContext(t *testing.T) {
	// Ensure role mention parses correctly when surrounded by text.
	res := testResolverFull()
	spans := Parse("hello <@&100> world", res)
	want := []Span{
		{Kind: Kind_Text, Text: "hello "},
		{Kind: Kind_RoleMention, Text: "@Moderator", FG: 0xFF5500, Action: &Action{Kind: ActionRoleMention, Target: "100"}},
		{Kind: Kind_Text, Text: " world"},
	}
	assertSpans(t, spans, want)
}

// ── Timestamps ───────────────────────────────────────────────────────────────

// fixedUnixSec is 2022-04-01 11:30:00 UTC, used across all timestamp tests.
const fixedUnixSec = 1648812600

func TestParseTimestampDefaultStyle(t *testing.T) {
	// <t:unix> with no style letter → "f" (short datetime).
	spans := Parse("<t:1648812600>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "01 April 2022 11:30")
}

func TestParseTimestampShortTime(t *testing.T) {
	spans := Parse("<t:1648812600:t>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "11:30")
}

func TestParseTimestampLongTime(t *testing.T) {
	spans := Parse("<t:1648812600:T>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "11:30:00")
}

func TestParseTimestampShortDate(t *testing.T) {
	spans := Parse("<t:1648812600:d>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "01/04/2022")
}

func TestParseTimestampLongDate(t *testing.T) {
	spans := Parse("<t:1648812600:D>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "01 April 2022")
}

func TestParseTimestampShortDateTime(t *testing.T) {
	spans := Parse("<t:1648812600:f>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "01 April 2022 11:30")
}

func TestParseTimestampFullDateTime(t *testing.T) {
	spans := Parse("<t:1648812600:F>", Resolver{})
	assertSingle(t, spans, Kind_Timestamp, "Friday, 01 April 2022 11:30")
}

func TestParseTimestampRelative_Past(t *testing.T) {
	// Arrange — Now is pinned to 2024-06-01 12:00:00, unix is 2022-04-01 12:30:00
	// → roughly 2 years = 730 days ago.
	res := testResolverFull()
	spans := Parse("<t:1648812600:R>", res)
	// Assert — we don't hard-code the exact label, just verify it contains "ago"
	// and is deterministic (same resolver → same output).
	if len(spans) != 1 || spans[0].Kind != Kind_Timestamp {
		t.Fatalf("spans = %+v", spans)
	}
	if !containsSuffix(spans[0].Text, " ago") {
		t.Errorf("relative past should end with ' ago', got %q", spans[0].Text)
	}
	// Run again; must produce identical text.
	spans2 := Parse("<t:1648812600:R>", res)
	if spans[0].Text != spans2[0].Text {
		t.Errorf("relative timestamp not deterministic: %q vs %q", spans[0].Text, spans2[0].Text)
	}
}

func TestParseTimestampRelative_Future(t *testing.T) {
	// Pin Now before the timestamp.
	res := testResolverFull()
	// 2024-06-01 12:00:00 UTC → 1717243200; timestamp = 1717243200 + 7200 = 1717250400 (2 h future)
	spans := Parse("<t:1717250400:R>", res)
	if len(spans) != 1 || spans[0].Kind != Kind_Timestamp {
		t.Fatalf("spans = %+v", spans)
	}
	if !containsPrefix(spans[0].Text, "in ") {
		t.Errorf("relative future should start with 'in ', got %q", spans[0].Text)
	}
}

func TestParseTimestampInvalidDegrades(t *testing.T) {
	// <t:notanumber> should not produce a timestamp span.
	spans := Parse("<t:notanumber>", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_Text {
		t.Fatalf("invalid timestamp should degrade to text, got %+v", spans)
	}
}

// ── Emoji ID / animated ──────────────────────────────────────────────────────

func TestParseEmojiCarriesID(t *testing.T) {
	spans := Parse("<:blob:123456789>", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_Emoji {
		t.Fatalf("spans = %+v", spans)
	}
	s := spans[0]
	if s.EmojiID != 123456789 {
		t.Errorf("EmojiID = %d, want 123456789", s.EmojiID)
	}
	if s.EmojiAnimated {
		t.Errorf("EmojiAnimated should be false for static emoji")
	}
	if s.Text != ":blob:" {
		t.Errorf("Text = %q, want :blob:", s.Text)
	}
}

func TestParseAnimatedEmojiCarriesFlag(t *testing.T) {
	spans := Parse("<a:spin:987>", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_Emoji {
		t.Fatalf("spans = %+v", spans)
	}
	s := spans[0]
	if !s.EmojiAnimated {
		t.Errorf("EmojiAnimated should be true for animated emoji")
	}
	if s.EmojiID != 987 {
		t.Errorf("EmojiID = %d, want 987", s.EmojiID)
	}
}

// ── Discord bare URL links ───────────────────────────────────────────────────

func TestParseChannelLink(t *testing.T) {
	// Arrange
	res := testResolverFull()
	url := "https://discord.com/channels/999/7"
	// Act
	spans := Parse(url, res)
	// Assert
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %+v", spans)
	}
	s := spans[0]
	if s.Kind != Kind_ChannelLink {
		t.Errorf("Kind = %v, want Kind_ChannelLink", s.Kind)
	}
	if s.Text != "#general" {
		t.Errorf("Text = %q, want #general", s.Text)
	}
	if s.Action == nil {
		t.Fatal("Action should not be nil")
	}
	if s.Action.Kind != ActionChannelLink {
		t.Errorf("Action.Kind = %v, want ActionChannelLink", s.Action.Kind)
	}
	if s.Action.Target != "999/7" {
		t.Errorf("Action.Target = %q, want 999/7", s.Action.Target)
	}
}

func TestParseChannelLinkUnknownChannel(t *testing.T) {
	// Arrange — no Channel resolver → falls back to "unknown-channel"
	spans := Parse("https://discord.com/channels/111/222", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_ChannelLink {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Text != "#unknown-channel" {
		t.Errorf("Text = %q, want #unknown-channel", spans[0].Text)
	}
	if spans[0].Action.Target != "111/222" {
		t.Errorf("Action.Target = %q, want 111/222", spans[0].Action.Target)
	}
}

func TestParseMessageLink(t *testing.T) {
	// Arrange
	res := testResolverFull()
	url := "https://discord.com/channels/999/7/88888"
	// Act
	spans := Parse(url, res)
	// Assert
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %+v", spans)
	}
	s := spans[0]
	if s.Kind != Kind_MessageLink {
		t.Errorf("Kind = %v, want Kind_MessageLink", s.Kind)
	}
	if s.Text != "#general ↷ 88888" {
		t.Errorf("Text = %q, want '#general ↷ 88888'", s.Text)
	}
	if s.Action == nil {
		t.Fatal("Action should not be nil")
	}
	if s.Action.Kind != ActionMessageLink {
		t.Errorf("Action.Kind = %v, want ActionMessageLink", s.Action.Kind)
	}
	if s.Action.Target != "999/7/88888" {
		t.Errorf("Action.Target = %q, want 999/7/88888", s.Action.Target)
	}
}

func TestParseMessageLinkInContext(t *testing.T) {
	// Link embedded in surrounding text.
	res := testResolverFull()
	spans := Parse("see https://discord.com/channels/999/7/88888 here", res)
	if len(spans) != 3 {
		t.Fatalf("want 3 spans, got %+v", spans)
	}
	if spans[0].Kind != Kind_Text || spans[0].Text != "see " {
		t.Errorf("span[0] = %+v", spans[0])
	}
	if spans[1].Kind != Kind_MessageLink {
		t.Errorf("span[1].Kind = %v, want Kind_MessageLink", spans[1].Kind)
	}
	if spans[2].Kind != Kind_Text || spans[2].Text != " here" {
		t.Errorf("span[2] = %+v", spans[2])
	}
}

func TestParseInviteLinkDiscordGG(t *testing.T) {
	// Arrange
	spans := Parse("https://discord.gg/abc123", Resolver{})
	// Assert
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %+v", spans)
	}
	s := spans[0]
	if s.Kind != Kind_InviteLink {
		t.Errorf("Kind = %v, want Kind_InviteLink", s.Kind)
	}
	if s.Text != "discord.gg/abc123" {
		t.Errorf("Text = %q, want discord.gg/abc123", s.Text)
	}
	if s.Action == nil || s.Action.Kind != ActionInvite {
		t.Errorf("Action = %+v", s.Action)
	}
	if s.Action.Target != "abc123" {
		t.Errorf("Action.Target = %q, want abc123", s.Action.Target)
	}
}

func TestParseInviteLinkDiscordComInvite(t *testing.T) {
	spans := Parse("https://discord.com/invite/XY-Z99", Resolver{})
	if len(spans) != 1 || spans[0].Kind != Kind_InviteLink {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Action.Target != "XY-Z99" {
		t.Errorf("Action.Target = %q, want XY-Z99", spans[0].Action.Target)
	}
}

func TestParseNonDiscordURLLeftAsPlainText(t *testing.T) {
	// Arbitrary https URL should not be consumed as a link.
	input := "https://example.com/foo"
	spans := Parse(input, Resolver{})
	// Should remain plain text (possibly a single text span or character-by-character).
	// The key invariant: no Discord-typed span should appear.
	for _, s := range spans {
		switch s.Kind {
		case Kind_ChannelLink, Kind_MessageLink, Kind_InviteLink:
			t.Errorf("non-Discord URL emitted Discord span %v", s.Kind)
		}
	}
}

func TestParseDiscordChannelLinkWithNonNumericID(t *testing.T) {
	// Non-numeric IDs should fall back to plain text.
	input := "https://discord.com/channels/abc/def"
	spans := Parse(input, Resolver{})
	for _, s := range spans {
		if s.Kind == Kind_ChannelLink || s.Kind == Kind_MessageLink {
			t.Errorf("non-numeric IDs should not produce a link span, got %v", s.Kind)
		}
	}
}

// ── Helper functions ──────────────────────────────────────────────────────────

func assertSingle(t *testing.T, spans []Span, kind Kind, text string) {
	t.Helper()
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d: %+v", len(spans), spans)
	}
	if spans[0].Kind != kind {
		t.Errorf("Kind = %v, want %v", spans[0].Kind, kind)
	}
	if spans[0].Text != text {
		t.Errorf("Text = %q, want %q", spans[0].Text, text)
	}
}

func spansEqual(a, b Span) bool {
	if a.Kind != b.Kind || a.Text != b.Text || a.URL != b.URL ||
		a.Format != b.Format || a.FG != b.FG || a.EmojiID != b.EmojiID || a.EmojiAnimated != b.EmojiAnimated {
		return false
	}
	if a.Action == nil && b.Action == nil {
		return true
	}
	if a.Action == nil || b.Action == nil {
		return false
	}
	return *a.Action == *b.Action
}

func assertSpans(t *testing.T, got, want []Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d spans, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !spansEqual(got[i], want[i]) {
			t.Errorf("span[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func containsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func containsPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
