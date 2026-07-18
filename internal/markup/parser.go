package markup

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/media"
)

// parser is a single-pass scanner over message content.
type parser struct {
	src    string
	res    Resolver
	format Format
	buf    strings.Builder
	spans  []Span
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
	if p.atLineStart(i) {
		switch {
		case p.has(i, "-#"):
			if n := p.scanSmall(i); n > i {
				return n
			}
		case p.has(i, "#"):
			if n := p.scanHeader(i); n > i {
				return n
			}
		case p.src[i] == '>':
			if n := p.scanQuote(i); n > i {
				return n
			}
		}
	}
	switch {
	case p.has(i, "```"):
		return p.scanFenced(i)
	case p.src[i] == '`':
		return p.scanInlineCode(i)
	case p.has(i, "***"):
		return p.scanDelimited(i, "***", FormatBold|FormatItalic)
	case p.has(i, "___"):
		return p.scanDelimited(i, "___", FormatUnderline|FormatItalic)
	case p.has(i, "**"):
		return p.scanDelimited(i, "**", FormatBold)
	case p.has(i, "__"):
		return p.scanDelimited(i, "__", FormatUnderline)
	case p.has(i, "~~"):
		return p.scanDelimited(i, "~~", FormatStrike)
	case p.has(i, "||"):
		return p.scanDelimited(i, "||", FormatSpoiler)
	case p.src[i] == '*':
		return p.scanDelimited(i, "*", FormatItalic)
	case p.src[i] == '_':
		return p.scanDelimited(i, "_", FormatItalic)
	case p.src[i] == '[':
		return p.scanLink(i)
	case p.src[i] == '<':
		return p.scanEntity(i)
	case p.has(i, "https://discord."):
		if n := p.scanDiscordURL(i); n > i {
			return n
		}
		return p.scanWebURL(i)
	case p.has(i, "https://"), p.has(i, "http://"):
		return p.scanWebURL(i)
	default:
		return i
	}
}

func (p *parser) scanWebURL(i int) int {
	end := urlEnd(p.src, i)
	if end <= i {
		return i
	}
	target := p.src[i:end]
	p.emit(Span{Kind: Kind_Link, Text: target, URL: target, Action: &Action{Kind: ActionOpenURL, Target: target}})
	return end
}

// scanSmall consumes Discord's -# small-text line. The marker requires a
// following space and the rest of the line remains available as plain text.
func (p *parser) scanSmall(i int) int {
	if i+3 >= len(p.src) || p.src[i:i+2] != "-#" || p.src[i+2] != ' ' {
		return i
	}
	start := i + 3
	end := lineEnd(p.src, start)
	p.emit(Span{Kind: Kind_Small, Text: p.src[start:end]})
	return end
}

func (p *parser) has(i int, s string) bool {
	return strings.HasPrefix(p.src[i:], s)
}

// atLineStart reports whether i begins a line (start of input or after \n).
func (p *parser) atLineStart(i int) bool {
	return i == 0 || p.src[i-1] == '\n'
}

// scanHeader consumes a Markdown heading up to level six. Plain headings keep
// their historical single Kind_Header span; headings containing inline markup
// emit a header marker followed by inline spans carrying the same level.
func (p *parser) scanHeader(i int) int {
	level := 0
	for level < 6 && i+level < len(p.src) && p.src[i+level] == '#' {
		level++
	}
	marker := i + level
	if level == 0 || marker >= len(p.src) || p.src[marker] != ' ' {
		return i
	}
	start := marker + 1
	end := lineEnd(p.src, start)
	inner := &parser{src: p.src[start:end], res: p.res, format: p.format}
	inner.run()
	if len(inner.spans) == 1 && inner.spans[0].Kind == Kind_Text {
		p.emit(Span{Kind: Kind_Header, Text: inner.spans[0].Text, HeaderLevel: level})
		return end
	}
	p.emit(Span{Kind: Kind_Header, HeaderLevel: level})
	for _, span := range inner.spans {
		span.HeaderLevel = level
		p.emit(span)
	}
	return end
}

// scanQuote consumes a "> " line quote, or ">>> " for a quote running to the
// end of the message. The gutter remains a Kind_Quote span, while the quoted
// body is parsed normally so entities such as custom emoji are preserved.
func (p *parser) scanQuote(i int) int {
	switch {
	case p.has(i, ">>> "):
		text := strings.TrimRight(p.src[i+4:], "\n")
		emitQuotedLines(p, text)
		return len(p.src)
	case p.has(i, "> "):
		end := lineEnd(p.src, i+2)
		emitQuotedLines(p, p.src[i+2:end])
		return end
	default:
		return i
	}
}

func emitQuotedLines(p *parser, text string) {
	for i, line := range strings.Split(text, "\n") {
		gutter := "▏ "
		if i > 0 {
			gutter = "\n▏ "
		}
		p.emit(Span{Kind: Kind_Quote, Text: gutter})
		inner := &parser{src: line, res: p.res, format: p.format}
		inner.run()
		for _, span := range inner.spans {
			span.Quoted = true
			p.spans = append(p.spans, span)
		}
	}
}

// lineEnd returns the index of the next newline at or after start, or len(s).
func lineEnd(s string, start int) int {
	if nl := strings.IndexByte(s[start:], '\n'); nl >= 0 {
		return start + nl
	}
	return len(s)
}

func (p *parser) flush() {
	if p.buf.Len() > 0 {
		p.spans = append(p.spans, formattedText(p.buf.String(), p.format))
		p.buf.Reset()
	}
}

func (p *parser) emit(s Span) {
	p.flush()
	s = p.applyFormat(s)
	p.spans = append(p.spans, s)
}

func (p *parser) applyFormat(s Span) Span {
	if p.format == 0 {
		return s
	}
	switch s.Kind {
	case Kind_Code, Kind_CodeBlock:
		return s
	}
	s.Format |= p.format
	return s
}

func formattedText(text string, format Format) Span {
	switch format {
	case 0:
		return Span{Kind: Kind_Text, Text: text}
	case FormatBold:
		return Span{Kind: Kind_Bold, Text: text}
	case FormatItalic:
		return Span{Kind: Kind_Italic, Text: text}
	case FormatUnderline:
		return Span{Kind: Kind_Underline, Text: text}
	case FormatStrike:
		return Span{Kind: Kind_Strike, Text: text}
	case FormatSpoiler:
		return Span{Kind: Kind_Spoiler, Text: text}
	default:
		return Span{Kind: Kind_Text, Text: text, Format: format}
	}
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

// scanDelimited consumes delim...delim, recursively parsing the inner content
// with the delimiter's formatting added to the current formatting set.
func (p *parser) scanDelimited(i int, delim string, format Format) int {
	start := i + len(delim)
	end := strings.Index(p.src[start:], delim)
	if end <= 0 { // require non-empty content
		return i
	}
	end += start
	p.flush()
	inner := &parser{
		src:    p.src[start:end],
		res:    p.res,
		format: p.format | format,
	}
	inner.run()
	p.spans = append(p.spans, inner.spans...)
	return end + len(delim)
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
	switch {
	case strings.HasPrefix(label, FakeEmojiMarker) && len(label) > len(FakeEmojiMarker) && media.ClassifyURL(url) == media.ClassEmoji:
		p.emit(Span{Kind: Kind_FakeEmoji, Text: strings.TrimPrefix(label, FakeEmojiMarker), URL: url})
		return urlStart + closeURL + 1
	case strings.HasPrefix(label, FakeStickerMarker) && len(label) > len(FakeStickerMarker) && media.ClassifyURL(url) == media.ClassSticker:
		p.emit(Span{Kind: Kind_FakeSticker, Text: strings.TrimPrefix(label, FakeStickerMarker), URL: url})
		return urlStart + closeURL + 1
	}
	p.emit(Span{Kind: Kind_Link, Text: label, URL: url, Action: &Action{Kind: ActionOpenURL, Target: url}})
	return urlStart + closeURL + 1
}

// scanEntity consumes a Discord entity: <@id>, <@!id>, <@&id>, <#id>,
// <:name:id>, <a:name:id>, <t:unix>, <t:unix:style>.
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
	case strings.HasPrefix(body, "@&"):
		return p.entityRoleMention(body[2:], next, i)
	case strings.HasPrefix(body, "@"):
		return p.entityMention(body[1:], next, i)
	case strings.HasPrefix(body, "#"):
		if id, ok := parseID(body[1:]); ok {
			p.emit(Span{Kind: Kind_ChannelMention, Text: "#" + p.res.channel(id)})
			return next
		}
	case strings.HasPrefix(body, "t:"):
		return p.entityTimestamp(body[2:], next, i)
	case strings.HasPrefix(body, ":") || strings.HasPrefix(body, "a:"):
		if name, id, animated, ok := emojiParts(body); ok {
			p.emit(Span{
				Kind:          Kind_Emoji,
				Text:          ":" + name + ":",
				EmojiID:       id,
				EmojiAnimated: animated,
			})
			return next
		}
	}
	return i
}

func (p *parser) entityMention(idStr string, next, fallback int) int {
	if id, ok := parseID(idStr); ok {
		p.emit(Span{Kind: Kind_Mention, Text: "@" + p.res.member(id), Action: &Action{Kind: ActionUserMention, Target: strconv.FormatUint(id, 10)}})
		return next
	}
	return fallback
}

// entityRoleMention handles <@&id> role mentions.
func (p *parser) entityRoleMention(idStr string, next, fallback int) int {
	if id, ok := parseID(idStr); ok {
		name, color := p.res.role(id)
		p.emit(Span{Kind: Kind_RoleMention, Text: "@" + name, FG: color, Action: &Action{Kind: ActionRoleMention, Target: strconv.FormatUint(id, 10)}})
		return next
	}
	return fallback
}

// entityTimestamp handles <t:unix> and <t:unix:style> timestamps.
// body is the content after the "t:" prefix.
func (p *parser) entityTimestamp(body string, next, fallback int) int {
	unix, style := body, "f"
	if idx := strings.IndexByte(body, ':'); idx >= 0 {
		unix = body[:idx]
		if s := body[idx+1:]; s != "" {
			style = s
		}
	}
	unixSec, ok := parseID(unix)
	if !ok {
		return fallback
	}
	t := time.Unix(int64(unixSec), 0).UTC()
	p.emit(Span{Kind: Kind_Timestamp, Text: formatTimestamp(t, style, p.res)})
	return next
}

// formatTimestamp converts a UTC time and a Discord style letter to a
// human-readable string. The style letters follow Discord's spec:
// t=short time, T=long time, d=short date, D=long date, f=short datetime
// (default), F=full datetime, R=relative.
func formatTimestamp(t time.Time, style string, res Resolver) string {
	switch style {
	case "t":
		return t.Format("15:04")
	case "T":
		return t.Format("15:04:05")
	case "d":
		return t.Format("02/01/2006")
	case "D":
		return t.Format("02 January 2006")
	case "F":
		return t.Format("Monday, 02 January 2006 15:04")
	case "R":
		return formatRelative(t, res.now())
	default: // "f" and unrecognised styles
		return t.Format("02 January 2006 15:04")
	}
}

// formatRelative returns a Discord-style relative time string such as
// "3 hours ago" or "in 2 days".
func formatRelative(t, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		return "in " + durationLabel(-d)
	}
	return durationLabel(d) + " ago"
}

// durationLabel returns a coarse human label for a duration (always positive).
func durationLabel(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "a few seconds"
	case d < 2*time.Minute:
		return "a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 2*time.Hour:
		return "an hour"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d < 48*time.Hour:
		return "a day"
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}

// scanDiscordURL tries to parse a bare Discord URL starting at i.
// Recognised patterns: discord.com/channels URLs and discord.gg/discord.com/invite
// invite links. All other bare URLs are left as plain text.
func (p *parser) scanDiscordURL(i int) int {
	end := urlEnd(p.src, i)
	if end == i {
		return i
	}
	raw := p.src[i:end]

	// discord.com/channels/<g>/<c>[/<m>]
	const chanPrefix = "https://discord.com/channels/"
	if strings.HasPrefix(raw, chanPrefix) {
		parts := strings.SplitN(raw[len(chanPrefix):], "/", 4)
		switch len(parts) {
		case 2:
			_, gok := parseID(parts[0])
			c, cok := parseID(parts[1])
			if gok && cok {
				p.emit(Span{
					Kind:   Kind_ChannelLink,
					Text:   "#" + p.res.channel(c),
					Action: &Action{Kind: ActionChannelLink, Target: parts[0] + "/" + parts[1]},
				})
				return end
			}
		case 3:
			_, gok := parseID(parts[0])
			c, cok := parseID(parts[1])
			_, mok := parseID(parts[2])
			if gok && cok && mok {
				target := parts[0] + "/" + parts[1] + "/" + parts[2]
				p.emit(Span{
					Kind:   Kind_MessageLink,
					Text:   "#" + p.res.channel(c) + " ↷ " + parts[2],
					Action: &Action{Kind: ActionMessageLink, Target: target},
				})
				return end
			}
		}
		return i
	}

	// discord.gg/<code> or discord.com/invite/<code>
	if code := extractInviteCode(raw); code != "" {
		p.emit(Span{
			Kind:   Kind_InviteLink,
			Text:   "discord.gg/" + code,
			Action: &Action{Kind: ActionInvite, Target: code},
		})
		return end
	}
	return i
}

// extractInviteCode returns the invite code for discord.gg and discord.com/invite
// URLs, or empty string if the URL is not an invite.
func extractInviteCode(raw string) string {
	var code string
	switch {
	case strings.HasPrefix(raw, "https://discord.gg/"):
		code = raw[len("https://discord.gg/"):]
	case strings.HasPrefix(raw, "https://discord.com/invite/"):
		code = raw[len("https://discord.com/invite/"):]
	default:
		return ""
	}
	if isValidInviteCode(code) {
		return code
	}
	return ""
}

// isValidInviteCode reports whether s is a plausible Discord invite code
// (non-empty, alphanumeric + hyphens only).
func isValidInviteCode(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// urlEnd returns the index just past the URL-like run starting at i.
func urlEnd(s string, i int) int {
	end := i
	for end < len(s) && isURLChar(s[end]) {
		end++
	}
	return end
}

// isURLChar reports whether b is a character that can appear inside a bare URL.
func isURLChar(b byte) bool {
	return b > ' ' && b != '"' && b != '\'' && b != '<' && b != '>' && b != '`' && b != '[' && b != ']'
}

func parseID(s string) (uint64, bool) {
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// emojiParts extracts name, snowflake ID, and animated flag from a Discord
// emoji body: ":name:id" or "a:name:id".
func emojiParts(body string) (name string, id uint64, animated bool, ok bool) {
	animated = strings.HasPrefix(body, "a")
	body = strings.TrimPrefix(body, "a")
	parts := strings.Split(body, ":") // ["", name, id]
	if len(parts) != 3 || parts[1] == "" {
		return "", 0, false, false
	}
	eid, eidOK := parseID(parts[2])
	if !eidOK {
		return "", 0, false, false
	}
	return parts[1], eid, animated, true
}
