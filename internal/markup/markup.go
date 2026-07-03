// Package markup parses Discord message content into a flat list of styled
// spans ready for terminal rendering.
//
// It handles the common inline markdown Discord supports — bold, italic, inline
// code, fenced code blocks, and links — plus Discord's own entity syntax:
// user mentions (<@id>, <@!id>), channel mentions (<#id>), and custom emoji
// (<:name:id>, <a:name:id>). Mentions and channels are resolved to display
// names through a Resolver; custom emoji render as :name: in v1.
//
// The parser is a single left-to-right pass. Code spans and code blocks
// suppress any inner parsing, matching Discord's behavior.
package markup

// Kind classifies a span so the renderer can style it.
type Kind int

const (
	// Text is unstyled body text.
	Kind_Text Kind = iota
	// Bold is **bold** text.
	Kind_Bold
	// Italic is *italic* or _italic_ text.
	Kind_Italic
	// Code is an inline `code` span.
	Kind_Code
	// CodeBlock is a fenced ```code block```.
	Kind_CodeBlock
	// Link is [label](url); Text is the label, URL the target.
	Kind_Link
	// Mention is a resolved @user.
	Kind_Mention
	// ChannelMention is a resolved #channel.
	Kind_ChannelMention
	// Emoji is a :name: custom emoji.
	Kind_Emoji
)

// Span is one contiguous, uniformly-styled run of rendered text.
type Span struct {
	Kind Kind
	Text string
	URL  string // set for Kind_Link
}

// Resolver looks up display names for Discord entity IDs. Either func may be nil;
// unknown IDs degrade to a readable placeholder.
type Resolver struct {
	Member  func(id uint64) (string, bool)
	Channel func(id uint64) (string, bool)
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

// Parse converts content into spans using res to resolve mentions and channels.
func Parse(content string, res Resolver) []Span {
	p := &parser{src: content, res: res}
	p.run()
	return p.spans
}
