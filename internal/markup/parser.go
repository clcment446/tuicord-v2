package markup

import (
	"strconv"
	"strings"
)

// parser is a single-pass scanner over message content.
type parser struct {
	src   string
	res   Resolver
	buf   strings.Builder
	spans []Span
}

func (p *parser) run() {
	for i := 0; i < len(p.src); {
		if n := p.matchAt(i); n > i {
			i = n
			continue
		}
		p.buf.WriteByte(p.src[i])
		i++
	}
	p.flush()
}

// matchAt tries every special construct at position i. It returns the index just
// past the consumed construct, or i if nothing matched.
func (p *parser) matchAt(i int) int {
	switch {
	case p.has(i, "```"):
		return p.scanFenced(i)
	case p.src[i] == '`':
		return p.scanInlineCode(i)
	case p.has(i, "**"):
		return p.scanDelimited(i, "**", Kind_Bold)
	case p.src[i] == '*':
		return p.scanDelimited(i, "*", Kind_Italic)
	case p.src[i] == '_':
		return p.scanDelimited(i, "_", Kind_Italic)
	case p.src[i] == '[':
		return p.scanLink(i)
	case p.src[i] == '<':
		return p.scanEntity(i)
	default:
		return i
	}
}

func (p *parser) has(i int, s string) bool {
	return strings.HasPrefix(p.src[i:], s)
}

func (p *parser) flush() {
	if p.buf.Len() > 0 {
		p.spans = append(p.spans, Span{Kind: Kind_Text, Text: p.buf.String()})
		p.buf.Reset()
	}
}

func (p *parser) emit(s Span) {
	p.flush()
	p.spans = append(p.spans, s)
}

// scanFenced consumes ```...``` (the opening/closing fences), emitting the inner
// text verbatim as a code block. No inner parsing happens.
func (p *parser) scanFenced(i int) int {
	start := i + 3
	end := strings.Index(p.src[start:], "```")
	if end < 0 {
		return i // unterminated; treat as literal
	}
	inner := p.src[start : start+end]
	inner = strings.Trim(inner, "\n")
	p.emit(Span{Kind: Kind_CodeBlock, Text: inner})
	return start + end + 3
}

func (p *parser) scanInlineCode(i int) int {
	end := strings.IndexByte(p.src[i+1:], '`')
	if end < 0 {
		return i
	}
	inner := p.src[i+1 : i+1+end]
	p.emit(Span{Kind: Kind_Code, Text: inner})
	return i + 1 + end + 1
}

// scanDelimited consumes delim...delim, emitting the inner text with kind.
func (p *parser) scanDelimited(i int, delim string, kind Kind) int {
	start := i + len(delim)
	end := strings.Index(p.src[start:], delim)
	if end <= 0 { // require non-empty content
		return i
	}
	inner := p.src[start : start+end]
	p.emit(Span{Kind: kind, Text: inner})
	return start + end + len(delim)
}

// scanLink consumes [label](url).
func (p *parser) scanLink(i int) int {
	closeLabel := strings.IndexByte(p.src[i:], ']')
	if closeLabel < 0 || i+closeLabel+1 >= len(p.src) || p.src[i+closeLabel+1] != '(' {
		return i
	}
	label := p.src[i+1 : i+closeLabel]
	urlStart := i + closeLabel + 2
	closeURL := strings.IndexByte(p.src[urlStart:], ')')
	if closeURL < 0 {
		return i
	}
	url := p.src[urlStart : urlStart+closeURL]
	p.emit(Span{Kind: Kind_Link, Text: label, URL: url})
	return urlStart + closeURL + 1
}

// scanEntity consumes a Discord entity: <@id>, <@!id>, <#id>, <:name:id>,
// <a:name:id>.
func (p *parser) scanEntity(i int) int {
	close := strings.IndexByte(p.src[i:], '>')
	if close < 0 {
		return i
	}
	body := p.src[i+1 : i+close] // between < and >
	next := i + close + 1

	switch {
	case strings.HasPrefix(body, "@!"):
		return p.entityMention(body[2:], next, i)
	case strings.HasPrefix(body, "@"):
		return p.entityMention(body[1:], next, i)
	case strings.HasPrefix(body, "#"):
		if id, ok := parseID(body[1:]); ok {
			p.emit(Span{Kind: Kind_ChannelMention, Text: "#" + p.res.channel(id)})
			return next
		}
	case strings.HasPrefix(body, ":") || strings.HasPrefix(body, "a:"):
		if name, ok := emojiName(body); ok {
			p.emit(Span{Kind: Kind_Emoji, Text: ":" + name + ":"})
			return next
		}
	}
	return i
}

func (p *parser) entityMention(idStr string, next, fallback int) int {
	if id, ok := parseID(idStr); ok {
		p.emit(Span{Kind: Kind_Mention, Text: "@" + p.res.member(id)})
		return next
	}
	return fallback
}

func parseID(s string) (uint64, bool) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// emojiName extracts the name from ":name:id" or "a:name:id".
func emojiName(body string) (string, bool) {
	body = strings.TrimPrefix(body, "a")
	parts := strings.Split(body, ":") // ["", name, id]
	if len(parts) != 3 || parts[1] == "" {
		return "", false
	}
	if _, ok := parseID(parts[2]); !ok {
		return "", false
	}
	return parts[1], true
}
