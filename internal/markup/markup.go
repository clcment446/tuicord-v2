// Package markup parses Discord message content into a flat list of styled
// spans ready for terminal rendering.
//
// It handles inline Discord markdown — bold, italic, inline code, fenced code
// blocks, links, underline, strikethrough, spoilers, blockquotes, and headings
// — plus Discord's entity syntax:
//
//   - user mentions (<@id>, <@!id>)
//   - role mentions (<@&id>)   → Kind_RoleMention, FG carries the role color
//   - channel mentions (<#id>)
//   - custom emoji (<:name:id>, <a:name:id>)
//   - timestamps (<t:unix>, <t:unix:style>)  → Kind_Timestamp
//
// Bare Discord URLs in body text are also recognised and emitted as typed
// spans with clickable Actions:
//
//   - discord.com/channels/g/c         → Kind_ChannelLink
//   - discord.com/channels/g/c/m       → Kind_MessageLink
//   - discord.gg/<code> / discord.com/invite/<code>  → Kind_InviteLink
//
// All other bare URLs are left as plain text (no auto-linking).
//
// Mentions, channels, roles, and guilds are resolved to display names through a
// Resolver; all Resolver fields are optional and degrade to readable
// placeholders.
//
// The parser is a single left-to-right pass. Code spans and code blocks
// suppress any inner parsing, matching Discord's behavior.
package markup

import "time"

// Kind classifies a span so the renderer can style it.
type Kind int

const (
	// Kind_Text is unstyled body text.
	Kind_Text Kind = iota
	// Kind_Bold is **bold** text.
	Kind_Bold
	// Kind_Italic is *italic* or _italic_ text.
	Kind_Italic
	// Kind_Code is an inline `code` span.
	Kind_Code
	// Kind_CodeBlock is a fenced ```code block```.
	Kind_CodeBlock
	// Kind_Link is [label](url); Text is the label, URL the target.
	Kind_Link
	// Kind_Mention is a resolved @user.
	Kind_Mention
	// Kind_ChannelMention is a resolved #channel entity (<#id>).
	Kind_ChannelMention
	// Kind_Emoji is a :name: custom emoji (<:name:id> or <a:name:id>).
	// EmojiID carries the snowflake; EmojiAnimated is set for the a: variant.
	Kind_Emoji
	// Kind_Underline is __underlined__ text.
	Kind_Underline
	// Kind_Strike is ~~struck-through~~ text.
	Kind_Strike
	// Kind_Spoiler is ||hidden|| text, revealed only on interaction.
	Kind_Spoiler
	// Kind_Quote is a > blockquote line.
	Kind_Quote
	// Kind_Header is a # / ## / ### heading line.
	Kind_Header
	// Kind_RoleMention is a <@&id> role mention. Text is "@RoleName"; FG is
	// the role's 0xRRGGBB color (0 = uncolored/inherit).
	Kind_RoleMention
	// Kind_MessageLink is a discord.com/channels/g/c/m URL rendered as a pill.
	// Action.Target is "guild/channel/message".
	Kind_MessageLink
	// Kind_ChannelLink is a discord.com/channels/g/c URL rendered as a pill.
	// Action.Target is "guild/channel".
	Kind_ChannelLink
	// Kind_InviteLink is a discord.gg/<code> or discord.com/invite/<code> URL.
	// Action.Target is the invite code.
	Kind_InviteLink
	// Kind_Timestamp is a <t:unix> or <t:unix:style> Discord timestamp entity.
	// Text is the pre-formatted, human-readable time string.
	Kind_Timestamp
)

// Format is the set of inline markdown formatting attributes applied to a
// span. It is used when formatting is stacked, or when formatting applies to a
// non-text span such as a mention.
type Format uint8

const (
	// FormatBold marks text wrapped in **...**.
	FormatBold Format = 1 << iota
	// FormatItalic marks text wrapped in *...* or _..._.
	FormatItalic
	// FormatUnderline marks text wrapped in __...__.
	FormatUnderline
	// FormatStrike marks text wrapped in ~~...~~.
	FormatStrike
	// FormatSpoiler marks text wrapped in ||...||.
	FormatSpoiler
)

// ActionKind identifies what clicking a span does.
type ActionKind int

const (
	// ActionOpenURL opens an arbitrary URL in the system browser.
	ActionOpenURL ActionKind = iota
	// ActionMessageLink jumps to or loads a specific message.
	// Action.Target is "guild/channel/message".
	ActionMessageLink
	// ActionChannelLink switches the active channel.
	// Action.Target is "guild/channel".
	ActionChannelLink
	// ActionInvite shows an invite preview or joins a server.
	// Action.Target is the invite code.
	ActionInvite
	// ActionUserMention opens the mentioned user's profile. Target is a user ID.
	ActionUserMention
	// ActionRoleMention opens role details/options. Target is a role ID.
	ActionRoleMention
)

// Action describes the clickable target attached to a span.
type Action struct {
	// Kind identifies the action type.
	Kind ActionKind
	// Target carries the action payload: a URL, "guild/channel",
	// "guild/channel/message", an invite code, or an entity ID, depending on Kind.
	Target string
}

// Span is one contiguous, uniformly-styled run of rendered text.
type Span struct {
	Kind Kind
	Text string
	URL  string // set for Kind_Link

	// Format carries stacked inline formatting attributes. Simple text-only
	// formatting keeps using Kind_Bold/Kind_Italic/etc for compatibility, so
	// Format is usually non-zero only for stacked styles or styled entities.
	Format Format

	// FG is a 0xRRGGBB foreground color override. 0 means inherit the theme
	// default. Currently set for Kind_RoleMention when the role has a color.
	FG uint32

	// EmojiID is the snowflake ID for Kind_Emoji custom emoji. 0 for standard
	// unicode emoji or when the ID is absent.
	EmojiID uint64
	// EmojiAnimated reports that the custom emoji uses the animated (a:) variant.
	EmojiAnimated bool

	// Action carries the clickable target for link and mention spans.
	// Nil if the span is not clickable.
	Action *Action
}

// Resolver looks up display names for Discord entity IDs. All fields are
// optional; nil functions and unknown IDs degrade to readable placeholders.
type Resolver struct {
	// Member resolves a user snowflake to a display name.
	Member func(id uint64) (string, bool)
	// Channel resolves a channel snowflake to a display name.
	Channel func(id uint64) (string, bool)
	// Role resolves a role snowflake to a display name and 0xRRGGBB color.
	// color 0 means uncolored.
	Role func(id uint64) (name string, color uint32, ok bool)
	// Guild resolves a guild snowflake to a server display name.
	Guild func(id uint64) (string, bool)
	// Now returns the current time used for relative timestamp rendering
	// (<t:unix:R>). When nil, time.Now is used. Inject a fixed value in tests
	// to keep relative-time output deterministic.
	Now func() time.Time
}

func (r Resolver) member(id uint64) string {
	if r.Member != nil {
		if name, ok := r.Member(id); ok {
			return name
		}
	}
	return "unknown-user"
}

func (r Resolver) channel(id uint64) string {
	if r.Channel != nil {
		if name, ok := r.Channel(id); ok {
			return name
		}
	}
	return "unknown-channel"
}

// role returns the display name and color for a role snowflake.
// Unknown roles degrade to "unknown-role" with color 0.
func (r Resolver) role(id uint64) (string, uint32) {
	if r.Role != nil {
		if name, color, ok := r.Role(id); ok {
			return name, color
		}
	}
	return "unknown-role", 0
}

// now returns the current time for relative timestamp rendering.
func (r Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Parse converts content into spans using res to resolve mentions and channels.
func Parse(content string, res Resolver) []Span {
	p := &parser{src: content, res: res}
	p.run()
	return p.spans
}
