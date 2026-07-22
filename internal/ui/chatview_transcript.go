package ui

import (
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"net/url"
	"strconv"
	"strings"
)

func (w *ChatView) ensureInitialFocusedMessage() {
	if w == nil || !w.keyboardFocused {
		return
	}
	latest, ok := w.store.LastMsg(w.active())
	if !ok {
		return
	}
	if w.focusedMessageSet && (w.focusedExplicit || sameMsg(w.focusedMessage, latest)) {
		return
	}
	previous := w.focusedMessage
	w.focusedMessage = latest
	w.focusedMessageSet = true
	w.focusKey = messagePlacementPrefix(w.focusedMessage)
	w.focusStopKey = ""
	w.invalidateMsgs(previous, latest)
}

func (w *ChatView) buildFocusIndex(lines []chatLine, height int) {
	for key := range w.focusRanges {
		delete(w.focusRanges, key)
	}
	w.focusStops = w.focusStops[:0]
	messageKey := ""
	var previous uint32
	bodySet := false
	for i, line := range lines {
		if line.msg == 0 {
			continue
		}
		if previous != line.msg {
			messageKey = messagePlacementPrefix(w.msgAt(line.msg))
			w.focusRanges[messageKey] = messageRange{start: i, end: i + 1}
			previous = line.msg
			bodySet = false
		} else {
			range_ := w.focusRanges[messageKey]
			range_.end = i + 1
			w.focusRanges[messageKey] = range_
		}
		firstBody := !line.author && !bodySet
		if firstBody {
			bodySet = true
		}
		if firstBody || line.embedStart {
			stop := chatFocusStop{kind: chatFocusMessage, key: messageKey + ":first", messageKey: messageKey, line: i, msg: line.msg}
			if line.embedKey != "" {
				stop.key = line.embedKey
			}
			if line.header != nil {
				stop.kind = chatFocusHeader
				stop.key = line.header.key
				stop.headerKey = line.header.key
			}
			w.focusStops = append(w.focusStops, stop)
		} else if line.header != nil {
			w.focusStops = append(w.focusStops, chatFocusStop{
				kind: chatFocusHeader, key: line.header.key, messageKey: messageKey,
				line: i, headerKey: line.header.key, msg: line.msg,
			})
		}
		for _, hit := range line.actions {
			w.focusStops = append(w.focusStops, chatFocusStop{
				kind: chatFocusControl, key: hit.key, messageKey: messageKey,
				action: hit.action,
				line:   i, start: hit.start, end: hit.end, msg: line.msg,
			})
		}
	}
	w.renderLineCount = len(lines)
	w.viewportHeight = height
	if len(w.focusStops) == 0 {
		w.focusKey = ""
		w.focusedMessageSet = false
		w.focusStopKey = ""
		w.focusStopIndex = -1
		return
	}
	selected := -1
	for i := range w.focusStops {
		if w.focusStops[i].key == w.focusStopKey {
			selected = i
			break
		}
	}
	if selected < 0 && w.focusedMessageSet {
		messageKey := w.focusKey
		for i := range w.focusStops {
			if w.focusStops[i].messageKey == messageKey {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		selected = len(w.focusStops) - 1
	}
	w.focusStopIndex = selected
	w.focusStopKey = w.focusStops[selected].key
	w.focusedMessage = w.msgAt(w.focusStops[selected].msg)
	w.focusKey = w.focusStops[selected].messageKey
	w.focusedMessageSet = true
}

func (w *ChatView) msgAt(msg uint32) store.Message {
	i := int(msg) - 1
	if i < 0 || i >= len(w.msgs) {
		return store.Message{}
	}
	return w.msgs[i]
}

// render turns the active channel's messages into wrapped, styled lines. Each
// message contributes a role-colored author line, its wrapped text content, and
// then any rich blocks: media chips, embeds, and a reactions line.
func (w *ChatView) render(width int) []chatLine {
	channel := w.active()
	msgRev := w.store.MsgRev(channel)
	metaRev := w.store.MetaRev()
	styleGen := w.styles.Version()
	w.renderGen++
	c := &w.transcript
	if c.stable && c.channel == channel && c.msgRev == msgRev && c.metaRev == metaRev &&
		c.componentEpoch == w.componentEpoch && c.styleGen == styleGen &&
		c.mediaEpoch == w.mediaEpoch && c.width == width {
		return c.lines
	}
	guild := w.guildOf(channel)
	w.msgs = w.store.MsgsInto(channel, w.msgs[:0])
	oldLen := len(c.lines)
	lines := c.lines[:0]
	stable := true
	var previous store.Message
	previousSet := false
	for i, m := range w.msgs {
		msg := uint32(i + 1)
		// The author line depends on the preceding message, so it is not a pure
		// function of m and stays outside the cache. It is one concat and a
		// color lookup, so recomputing it is cheaper than tracking it.
		showAuthor := !previousSet || !sameMessageAuthor(previous, m) ||
			previous.Failed != m.Failed || previous.Pending != m.Pending
		if showAuthor {
			line := w.authorLine(m, guild)
			line.msg = msg
			lines = append(lines, line)
		}
		body, ok := w.cachedBody(m, channel, width)
		if !ok {
			body, ok = w.renderBody(m, channel, width)
		}
		stable = stable && ok
		start := len(lines)
		lines = append(lines, body...)
		for i := start; i < len(lines); i++ {
			lines[i].msg = msg
		}
		previous = m
		previousSet = true
	}
	if oldLen > len(lines) {
		clear(c.lines[len(lines):oldLen])
	}
	c.lines = lines
	c.channel = channel
	c.msgRev = msgRev
	c.metaRev = metaRev
	c.componentEpoch = w.componentEpoch
	c.styleGen = styleGen
	c.mediaEpoch = w.mediaEpoch
	c.width = width
	c.stable = stable && !(w.roleGradientAnimations && hasRoleGradient(lines))
	c.gen++
	w.sweepBodyCache()
	w.sweepMedia()
	return c.lines
}

// renderBody renders and caches everything a message contributes below its
// author line.
//
// The emoji placement counters are reset here, in the miss path, rather than
// once per message in render. Every placement key a body emits is prefixed with
// that message's own prefix and numbered from zero, so a body's keys depend
// only on the message — never on which neighbours were cache hits. That is what
// makes skipping a message safe.
func (w *ChatView) renderBody(m store.Message, channel store.ChannelID, width int) ([]chatLine, bool) {
	w.emojiKeyPrefix = messagePlacementPrefix(m)
	w.emojiSeq = 0
	w.headerMessageKey = messagePlacementPrefix(m)
	w.headerSeq = 0
	style := w.styles.Cell("messages.content")
	switch {
	case m.Failed:
		style = w.styles.Cell("messages.failed")
	case m.Pending:
		style = w.styles.Cell("messages.pending")
	}

	w.bodyDeps = w.bodyDeps[:0]
	w.collectDeps = true

	var body []chatLine
	if m.Reply != nil {
		body = append(body, w.renderReplyLine(*m.Reply, channel, width))
	}
	if m.Content != "" && !suppressContent(m) {
		body = append(body, w.renderContent(m.Content, width, style)...)
	}
	body = append(body, w.renderForwards(m, width, style)...)
	body = append(body, w.renderMedia(m, width, style)...)
	body = append(body, w.renderEmbeds(m, width, style)...)
	body = append(body, w.renderComponentTree(m, width, style)...)
	if line, ok := w.renderReactions(m.Reactions, messagePlacementPrefix(m)); ok {
		body = append(body, line)
	}
	if line, ok := w.renderThreadStarter(m, channel); ok {
		body = append(body, line)
	}

	w.collectDeps = false
	return body, w.storeBody(m, channel, width, body)
}

// cachedBody returns a previously rendered body when every input it depended on
// is unchanged.
func (w *ChatView) cachedBody(m store.Message, channel store.ChannelID, width int) ([]chatLine, bool) {
	e := w.bodyCache[messagePlacementPrefix(m)]
	if e == nil {
		return nil, false
	}
	if e.rev != m.Rev() || e.width != width || e.channel != channel ||
		e.metaRev != w.store.MetaRev() ||
		e.styleGeneration != w.styles.Version() {
		return nil, false
	}
	for _, d := range e.deps {
		state := w.media[d.url]
		if state == nil || state.rev != d.rev {
			return nil, false
		}
	}
	// A hit skips renderBody, so ensureMedia never runs for this body. Mark its
	// media as still in use here or the sweep would evict it and every
	// subsequent frame would miss and re-render.
	for _, d := range e.deps {
		w.media[d.url].touched = w.renderGen
	}
	e.gen = w.renderGen
	return e.lines, true
}

// storeBody memoizes a rendered body, unless it drew a spinner. A loading body
// animates with w.spinner, which is deliberately not part of the cache key:
// including it would invalidate every entry twice a second and defeat the
// cache. Not caching the few loading bodies costs little and keeps the spinner
// moving.
func (w *ChatView) storeBody(m store.Message, channel store.ChannelID, width int, body []chatLine) bool {
	for _, d := range w.bodyDeps {
		// Loading bodies animate a spinner; animated bodies swap frames each tick.
		// Caching either would freeze that motion, so leave them uncached.
		if state := w.media[d.url]; state != nil && (state.loading || state.animated()) {
			return false
		}
	}
	if w.bodyCache == nil {
		w.bodyCache = map[string]*chatCacheEntry{}
	}
	deps := append([]mediaDep(nil), w.bodyDeps...)
	w.bodyCache[messagePlacementPrefix(m)] = &chatCacheEntry{
		lines:           body,
		rev:             m.Rev(),
		width:           width,
		channel:         channel,
		metaRev:         w.store.MetaRev(),
		styleGeneration: w.styles.Version(),
		deps:            deps,
		gen:             w.renderGen,
	}
	return true
}

// sweepBodyCache drops entries no recent render touched. Entries for other
// channels survive until the budget is reached, so flipping between two
// channels stays free.
func (w *ChatView) sweepBodyCache() {
	if len(w.bodyCache) <= maxBodyCache {
		return
	}
	for key, e := range w.bodyCache {
		if e.gen != w.renderGen {
			delete(w.bodyCache, key)
		}
	}
}

// sweepMedia drops media no recent render read. Without it w.media grows for
// the lifetime of the session, holding a decoded image for every URL seen in
// every channel visited.
//
// Evicting is safe for the body cache: a cached body whose media disappears
// fails its dependency check and re-renders. In-flight fetches are kept because
// their goroutine still expects the state it incremented the loading count for.
func (w *ChatView) sweepMedia() {
	if len(w.media) <= maxMediaStates {
		return
	}
	for url, state := range w.media {
		if state.touched != w.renderGen && !state.loading {
			delete(w.media, url)
			w.mediaEpoch++
		}
	}
}

func hasRoleGradient(lines []chatLine) bool {
	for i := range lines {
		if lines[i].roleGradient {
			return true
		}
	}
	return false
}

// invalidateBodies drops every memoized body. Used when presentation state
// changes in a way that is not worth versioning precisely.
func (w *ChatView) invalidateBodies() {
	w.componentEpoch++
	clear(w.bodyCache)
}

func (w *ChatView) invalidateMsgs(messages ...store.Message) {
	w.componentEpoch++
	for _, message := range messages {
		if message.ID != 0 || message.Nonce != "" {
			delete(w.bodyCache, messagePlacementPrefix(message))
		}
	}
}

func (w *ChatView) authorLine(m store.Message, guild store.GuildID) chatLine {
	header := m.Author
	if m.Failed {
		header += " (failed)"
	} else if m.Pending {
		header += " (sending…)"
	}
	authorStyle := w.styles.Cell("messages.author")
	if m.Failed {
		authorStyle = mergeStyle(authorStyle, w.styles.Cell("messages.failed"))
	} else if m.Pending {
		authorStyle = mergeStyle(authorStyle, w.styles.Cell("messages.pending"))
	}
	if color := w.store.MemberColor(guild, m.AuthorID); color != 0 && !w.styles.HasCustom("messages.author") {
		authorStyle.Fg = rgbColor(color)
	}
	role, gradient := w.store.MemberDisplayRole(guild, m.AuthorID)
	gradient = gradient && role.Colors[1] != 0 && w.roleGradients && !w.styles.HasCustom("messages.author")
	avatarURL := m.AuthorAvatarURL
	if member, ok := w.store.Member(guild, m.AuthorID); ok && member.AvatarURL != "" {
		avatarURL = member.AvatarURL
	}
	if !gradient && (avatarURL == "" || !w.mediaCfg.Enabled) {
		return chatLine{text: header, style: authorStyle, author: true}
	}
	segments := []chatSegment(nil)
	if gradient {
		denominator := float64(len([]rune(header)))
		if denominator < 1 {
			denominator = 1
		}
		index := 0
		for cluster := range text.Clusters(header) {
			position := (float64(index) + w.roleGradientPhase) / denominator
			style := authorStyle
			style.Fg = rgbColor(role.GradientAt(position - float64(int(position))))
			segments = appendChatSegment(segments, chatSegment{text: cluster.Text, style: style})
			index++
		}
	}
	line := chatLine{
		segments:     segments,
		author:       true,
		roleGradient: gradient,
	}
	if !gradient {
		line.segments = []chatSegment{{text: "  | " + header, style: authorStyle}}
	} else if avatarURL != "" && w.mediaCfg.Enabled {
		line.segments = append([]chatSegment{{text: "  | ", style: authorStyle}}, line.segments...)
	}
	if avatarURL == "" || !w.mediaCfg.Enabled {
		return line
	}
	if state := w.ensureMedia(avatarURL, false); state != nil && state.err == nil && state.img != nil {
		variant := w.mediaVariant(state, mediaSpec{maxCols: 2, maxRows: 1, sourceW: 48, sourceH: 48, square: true})
		line.inlineMedia = []positionedInlineMedia{{
			col:   0,
			media: &inlineMedia{url: avatarURL, placementKey: messagePlacementPrefix(m) + ":avatar", cols: variant.cols, rows: variant.rows, img: variant.img, style: authorStyle},
		}}
	}
	return line
}

func sameMessageAuthor(a, b store.Message) bool {
	if a.AuthorID != 0 || b.AuthorID != 0 {
		return a.AuthorID != 0 && a.AuthorID == b.AuthorID
	}
	return a.Author == b.Author
}

func sameMsg(a, b store.Message) bool {
	return a.ChannelID == b.ChannelID && a.ID == b.ID && a.Nonce == b.Nonce
}

// renderThreadStarter emits a "⤷ thread-name (N messages)" line under a message
// that started a thread. Discord gives a message-anchored thread the same
// snowflake as its anchor message, so the thread is found by the message ID.
func (w *ChatView) renderThreadStarter(m store.Message, channel store.ChannelID) (chatLine, bool) {
	c, ok := w.store.Channel(store.ChannelID(m.ID))
	if !ok || c.Kind != store.ChannelThread || c.ParentID != channel {
		return chatLine{}, false
	}
	text := "  ⤷ " + c.Name
	if c.Thread != nil && c.Thread.MessageCount > 0 {
		text += " (" + strconv.Itoa(c.Thread.MessageCount) + " messages)"
	}
	return chatLine{text: text, style: w.styles.Cell("messages.thread")}, true
}

// guildOf reports the guild that owns a channel, or 0 when unknown.
func (w *ChatView) guildOf(channel store.ChannelID) store.GuildID {
	if c, ok := w.store.Channel(channel); ok {
		return c.GuildID
	}
	return 0
}

func (w *ChatView) renderContent(content string, width int, base screen.Style) []chatLine {
	var res markup.Resolver
	if w.resolver != nil {
		res = w.resolver()
	}
	var lines []chatLine
	var line []chatSegment
	var entities []entityHit
	var inline []positionedInlineMedia
	spinner := false
	collapsedLevel := 0
	skip := false
	skipHeaderNewline := false
	var pendingHeader *chatLine
	used := 0
	flush := func() {
		lines = append(lines, chatLine{segments: line, inlineMedia: inline, entities: entities, spinner: spinner})
		line = nil
		entities = nil
		inline = nil
		spinner = false
		used = 0
	}
	appendText := func(s string, style screen.Style, action *markup.Action) {
		parts := strings.Split(s, "\n")
		for i, part := range parts {
			if i > 0 {
				if skipHeaderNewline && len(line) == 0 && len(inline) == 0 {
					skipHeaderNewline = false
				} else {
					flush()
				}
			}
			for cluster := range text.Clusters(part) {
				if cluster.Width == 0 {
					continue
				}
				if width > 0 && used > 0 && used+cluster.Width > width {
					flush()
				}
				line = appendChatSegment(line, chatSegment{text: cluster.Text, style: style})
				if action != nil {
					entities = append(entities, entityHit{start: used, end: used + cluster.Width, action: *action})
				}
				used += cluster.Width
			}
		}
	}
	for _, span := range markup.Parse(content, res) {
		if pendingHeader != nil && span.Kind != markup.Kind_Header {
			if span.HeaderLevel > 0 {
				style := w.markupStyle(span, base)
				for cluster := range text.Clusters(span.Text) {
					if cluster.Width == 0 || (width > 0 && chatLineWidth(*pendingHeader)+cluster.Width > width) {
						continue
					}
					pendingHeader.segments = appendChatSegment(pendingHeader.segments, chatSegment{text: cluster.Text, style: style})
				}
				continue
			}
			lines = append(lines, *pendingHeader)
			pendingHeader = nil
		}
		if span.Kind == markup.Kind_Header {
			level := span.HeaderLevel
			key := w.headerMessageKey + ":header:" + strconv.Itoa(w.headerSeq)
			w.headerSeq++
			if collapsedLevel != 0 {
				if level > collapsedLevel {
					continue
				}
				collapsedLevel = 0
				skip = false
			}
			style := w.markupStyle(span, base)
			collapsed := w.collapsedHeaders[key]
			marker := "▾ "
			if collapsed {
				marker = "▸ "
			}
			if len(line) > 0 || len(inline) > 0 {
				flush()
			}
			headerLine := chatLine{header: &headerHit{key: key, level: level, collapsed: collapsed}}
			for cluster := range text.Clusters(text.Truncate(marker+span.Text, width, text.Ellipsis)) {
				if cluster.Width == 0 {
					continue
				}
				headerLine.segments = appendChatSegment(headerLine.segments, chatSegment{text: cluster.Text, style: style})
			}
			if span.Text == "" {
				pendingHeader = &headerLine
			} else {
				lines = append(lines, headerLine)
			}
			skipHeaderNewline = true
			skip = collapsed
			if collapsed {
				collapsedLevel = level
			}
			continue
		}
		if skip {
			continue
		}
		style := w.markupStyle(span, base)
		mentionStyle := ""
		if span.Kind == markup.Kind_RoleMention {
			mentionStyle = "messages.roleMention"
		} else if span.Kind == markup.Kind_Mention || span.Kind == markup.Kind_ChannelMention {
			mentionStyle = "messages.mention"
		}
		if span.FG != 0 && (mentionStyle == "" || !w.styles.HasCustom(mentionStyle)) {
			style.Fg = rgbColor(span.FG)
		}
		if span.Kind == markup.Kind_FakeSticker {
			if media.ClassifyURL(span.URL) != media.ClassSticker {
				appendText(span.Text, style, nil)
				continue
			}
			if len(line) > 0 {
				flush()
			}
			w.emojiSeq++
			lines = append(lines, w.mediaLines(
				span.URL,
				"[sticker: "+span.Text+"]",
				w.emojiKeyPrefix+":sticker:"+strconv.Itoa(w.emojiSeq)+":"+span.URL,
				style,
				stickerMediaSpec(width),
				false,
			)...)
			continue
		}

		emojiURL := ""
		switch {
		case span.Kind == markup.Kind_Emoji && span.EmojiID != 0:
			emojiURL = customEmojiURL(span)
		case span.Kind == markup.Kind_FakeEmoji && media.ClassifyURL(span.URL) == media.ClassEmoji:
			emojiURL = span.URL
		}
		if emojiURL != "" && w.mediaCfg.Enabled && w.mediaCfg.EmojiImages {
			const emojiCols = 2
			if width > 0 && used > 0 && used+emojiCols > width {
				flush()
			}
			state := w.ensureMedia(emojiURL, false)
			placeholder := strings.Repeat(" ", emojiCols)
			if state != nil && state.loading {
				placeholder = mediaSpinner(w.spinner) + " "
				spinner = true
			}
			line = appendChatSegment(line, chatSegment{text: placeholder, style: style})
			if state != nil && state.err == nil && state.img != nil {
				variant := w.mediaVariant(state, mediaSpec{
					maxCols: emojiCols,
					maxRows: 1,
					sourceW: 48,
					sourceH: 48,
					square:  true,
				})
				w.emojiSeq++
				inline = append(inline, positionedInlineMedia{
					col: used,
					media: &inlineMedia{
						url:          emojiURL,
						label:        span.Text,
						placementKey: w.emojiKeyPrefix + ":emoji:" + strconv.Itoa(w.emojiSeq) + ":" + emojiURL,
						cols:         emojiCols,
						rows:         1,
						img:          variant.img,
						style:        style,
					},
				})
			}
			used += emojiCols
			continue
		}
		if span.Kind == markup.Kind_FakeEmoji {
			appendText(":"+span.Text+":", style, nil)
			continue
		}
		appendText(span.Text, style, span.Action)
	}
	if pendingHeader != nil {
		lines = append(lines, *pendingHeader)
	}
	if len(line) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func customEmojiURL(span markup.Span) string {
	return customEmojiURLParts(span.EmojiID, strings.Trim(span.Text, ":"), span.EmojiAnimated)
}

func customEmojiURLParts(id uint64, name string, animated bool) string {
	ext := "webp"
	if animated {
		ext = "gif"
	}
	return "https://cdn.discordapp.com/emojis/" +
		strconv.FormatUint(id, 10) + "." + ext +
		"?size=48&name=" + url.QueryEscape(name) + "&lossless=true"
}

func (w *ChatView) markupStyle(span markup.Span, base screen.Style) screen.Style {
	style := base
	if span.Quoted {
		style = mergeStyle(style, w.styles.Cell("messages.quote"))
		style.Bg = base.Bg
	}
	switch span.Kind {
	case markup.Kind_Bold:
		style = mergeStyle(style, w.styles.Cell("messages.bold"))
	case markup.Kind_Italic:
		style = mergeStyle(style, w.styles.Cell("messages.italic"))
	case markup.Kind_Code, markup.Kind_CodeBlock:
		style = mergeStyle(style, w.styles.Cell("messages.code"))
	case markup.Kind_Underline:
		style = mergeStyle(style, w.styles.Cell("messages.underlined"))
	case markup.Kind_Strike:
		style = mergeStyle(style, w.styles.Cell("messages.strikethrough"))
	case markup.Kind_Spoiler:
		// No hover in a TUI, so mask the text as a reverse-video block.
		style = mergeStyle(style, w.styles.Cell("messages.spoiler"))
	case markup.Kind_Link:
		style = mergeStyle(style, w.styles.Cell("messages.link.prettyLink"))
	case markup.Kind_Quote:
		style = mergeStyle(style, w.styles.Cell("messages.quote"))
		style.Bg = base.Bg
	case markup.Kind_Header:
		level := span.HeaderLevel
		if level < 1 || level > 6 {
			level = 1
		}
		style = mergeStyle(style, w.styles.Cell("messages.header"+strconv.Itoa(level)))
	case markup.Kind_Small:
		style = mergeStyle(style, w.styles.Cell("messages.small"))
	case markup.Kind_Mention, markup.Kind_ChannelMention:
		style = mergeStyle(style, w.styles.Cell("messages.mention"))
	case markup.Kind_RoleMention:
		style = mergeStyle(style, w.styles.Cell("messages.roleMention"))
	case markup.Kind_MessageLink, markup.Kind_ChannelLink, markup.Kind_InviteLink:
		name := "messages.link.invite"
		if span.Kind == markup.Kind_MessageLink {
			name = "messages.link.message"
		} else if span.Kind == markup.Kind_ChannelLink {
			name = "messages.link.channel"
		}
		style = mergeStyle(style, w.styles.Cell(name))
	case markup.Kind_Timestamp:
		style = mergeStyle(style, w.styles.Cell("messages.timestamp"))
	}
	if span.HeaderLevel > 0 {
		level := span.HeaderLevel
		if level > 6 {
			level = 6
		}
		style = mergeStyle(style, w.styles.Cell("messages.header"+strconv.Itoa(level)))
	}
	if span.Format&markup.FormatBold != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.bold"))
	}
	if span.Format&markup.FormatItalic != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.italic"))
	}
	if span.Format&markup.FormatUnderline != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.underlined"))
	}
	if span.Format&markup.FormatStrike != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.strikethrough"))
	}
	if span.Format&markup.FormatSpoiler != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.spoiler"))
	}
	return style
}

// rgbColor converts a 0xRRGGBB value into a screen color.
func rgbColor(c uint32) screen.Color {
	return screen.RGB(uint8(c>>16), uint8(c>>8), uint8(c))
}

func appendChatSegment(segments []chatSegment, next chatSegment) []chatSegment {
	if next.text == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].style == next.style {
		segments[len(segments)-1].text += next.text
		return segments
	}
	return append(segments, next)
}

func drawChatLine(r screen.Region, x, y int, line chatLine) {
	if len(line.segments) == 0 {
		drawText(r, x, y, line.text, line.style)
		return
	}
	for _, segment := range line.segments {
		x = drawText(r, x, y, segment.text, segment.style)
		if x >= r.Width() {
			return
		}
	}
}

func drawFocusedChatLine(r screen.Region, x, y int, line chatLine, focusStart, focusEnd int, focus screen.Style, fillFocus bool) {
	if line.restrictHighlight {
		focusStart = max(focusStart, line.highlightStart)
		focusEnd = min(focusEnd, line.highlightEnd)
	}
	segments := line.segments
	if len(segments) == 0 {
		segments = []chatSegment{{text: line.text, style: line.style}}
	}
	// Segmented lines, notably markdown headers, carry their semantic color on
	// the segment rather than chatLine.style. Use that style for the trailing
	// focus fill so the whole row shares the header color.
	focusBase := line.style
	if len(segments) > 0 {
		focusBase = segments[0].style
	}
	col := x
	for _, segment := range segments {
		for cluster := range text.Clusters(segment.text) {
			if cluster.Width <= 0 || col >= r.Width() {
				continue
			}
			style := segment.style
			if col < focusEnd && col+cluster.Width > focusStart {
				style = Styles{}.focusedStyle(style)
				if focus.Fg.Set() || focus.Bg.Set() {
					style = mergeStyle(style, focus)
					style.Attrs &^= screen.Reverse
				}
			}
			r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
			col += cluster.Width
		}
	}
	if fillFocus {
		style := Styles{}.focusedStyle(focusBase)
		if focus.Fg.Set() || focus.Bg.Set() {
			style = mergeStyle(style, focus)
			style.Attrs &^= screen.Reverse
		}
		for col < min(focusEnd, r.Width()) {
			if col >= focusStart {
				r.Set(col, y, screen.Cell{Content: " ", Style: style})
			}
			col++
		}
	}
}
