package widget

import (
	"path/filepath"
	"strings"

	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// MarkupKind describes the source kind of a parsed span.
type MarkupKind int

const (
	// MarkupText is ordinary text.
	MarkupText MarkupKind = iota
	// MarkupLink is a [label](target) link.
	MarkupLink
	// MarkupFile is a [label](./path) local file attachment.
	MarkupFile
	// MarkupImage is a ![label](./path) image attachment.
	MarkupImage
)

// MarkupSpan is a parsed run of markup text.
type MarkupSpan struct {
	// Text is the plain text to draw.
	Text string
	// Kind identifies plain text, links, files, and images.
	Kind MarkupKind
	// Bold reports whether the span should draw bold.
	Bold bool
	// Italic reports whether the span should draw italic.
	Italic bool
	// Underline reports whether the span should draw underlined.
	Underline bool
	// Strike reports whether the span should draw struck through.
	Strike bool
	// Link is the link target from [text](target). It may be empty for
	// markdown's valid [text]() form.
	Link string
}

// Markup draws a small markdown-like subset as styled spans.
//
// Supported syntax is intentionally narrow: [label](target), local file links,
// image attachments, bold/italic emphasis, underline, and strikethrough. Any
// malformed opener is rendered literally instead of being dropped.
type Markup struct {
	source    string
	spans     []MarkupSpan
	style     screen.Style
	boldStyle screen.Style
	linkStyle screen.Style
	wrap      bool
	node      layout.Node
}

// NewMarkup returns a markup widget containing source.
func NewMarkup(source string) *Markup {
	w := &Markup{
		wrap:      true,
		boldStyle: screen.Style{Attrs: screen.Bold},
		linkStyle: screen.Style{Attrs: screen.Underline},
		node:      layout.Node{Grow: 1},
	}
	w.SetSource(source)
	return w
}

// Source returns the raw markup source.
func (w *Markup) Source() string {
	if w == nil {
		return ""
	}
	return w.source
}

// SetSource replaces and reparses the markup source.
func (w *Markup) SetSource(source string) {
	if w == nil {
		return
	}
	w.source = source
	w.spans = parseMarkup(source)
}

// Spans returns a copy of the parsed spans.
func (w *Markup) Spans() []MarkupSpan {
	if w == nil {
		return nil
	}
	out := make([]MarkupSpan, len(w.spans))
	copy(out, w.spans)
	return out
}

// SetStyle sets the base text style.
func (w *Markup) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetBoldStyle sets the style merged onto bold spans.
func (w *Markup) SetBoldStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.boldStyle = style
}

// SetLinkStyle sets the style merged onto link label spans.
func (w *Markup) SetLinkStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.linkStyle = style
}

// SetWrap controls whether markup wraps to the available width.
func (w *Markup) SetWrap(wrap bool) {
	if w == nil {
		return
	}
	w.wrap = wrap
}

// Measure returns the rendered plain-text size within avail.
func (w *Markup) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	lines := w.renderLines(avail.W)
	width := 0
	for _, line := range lines {
		width = maxInt(width, spansWidth(line))
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	return tui.Size{W: width, H: len(lines)}
}

// Layout returns the layout node for this markup widget.
func (w *Markup) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders parsed markup into r.
func (w *Markup) Draw(r screen.Region) {
	if w == nil {
		return
	}
	clear(r, w.style)
	lines := w.renderLines(r.Width())
	for y, line := range lines {
		if y >= r.Height() {
			break
		}
		clearLine(r, y, w.style)
		x := 0
		for _, span := range line {
			x = drawText(r, x, y, span.Text, w.spanStyle(span))
			if x >= r.Width() {
				break
			}
		}
	}
}

// Handle ignores input events and reports them unconsumed.
func (w *Markup) Handle(tui.Event) bool {
	return false
}

func (w *Markup) renderLines(width int) [][]MarkupSpan {
	if width <= 0 || !w.wrap {
		return splitMarkupLines(w.spans)
	}
	var lines [][]MarkupSpan
	var line []MarkupSpan
	used := 0
	flush := func() {
		lines = append(lines, line)
		line = nil
		used = 0
	}
	for _, span := range w.spans {
		parts := strings.Split(span.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				flush()
			}
			for cluster := range text.Clusters(part) {
				if cluster.Width == 0 {
					continue
				}
				if used > 0 && used+cluster.Width > width {
					flush()
				}
				line = appendSpanText(line, span, cluster.Text)
				used += cluster.Width
			}
		}
	}
	return append(lines, line)
}

func (w *Markup) spanStyle(span MarkupSpan) screen.Style {
	style := w.style
	if span.Bold {
		style = mergeStyle(style, w.boldStyle)
	}
	if span.Italic {
		style.Attrs |= screen.Italic
	}
	if span.Underline {
		style.Attrs |= screen.Underline
	}
	if span.Strike {
		style.Attrs |= screen.Strike
	}
	if span.Kind == MarkupLink || span.Kind == MarkupFile || span.Kind == MarkupImage {
		style = mergeStyle(style, w.linkStyle)
	}
	return style
}

func parseMarkup(source string) []MarkupSpan {
	spans, _ := parseMarkupUntil(source, MarkupSpan{}, "")
	return compactSpans(spans)
}

func parseMarkupUntil(source string, style MarkupSpan, closer string) ([]MarkupSpan, string) {
	var spans []MarkupSpan
	for len(source) > 0 {
		if closer != "" && strings.HasPrefix(source, closer) {
			return spans, source[len(closer):]
		}
		if triple, rest, ok := parseTripleMixed(source, style); ok {
			spans = append(spans, triple...)
			source = rest
			continue
		}
		if span, rest, ok := parseLinkedSpan(source, style); ok {
			spans = append(spans, span)
			source = rest
			continue
		}
		if open, close, nextStyle, ok := emphasis(source, style); ok {
			inner, rest := parseMarkupUntil(source[len(open):], nextStyle, close)
			if rest == "" && !strings.Contains(source[len(open):], close) {
				nextStyle.Text = source[len(open):]
				spans = append(spans, nextStyle)
				return spans, ""
			}
			spans = append(spans, inner...)
			source = rest
			continue
		}
		next := nextMarkupStart(source)
		if next <= 0 {
			next = len(source)
		}
		plain := style
		plain.Text = source[:next]
		spans = append(spans, plain)
		source = source[next:]
	}
	return spans, ""
}

func parseTripleMixed(source string, style MarkupSpan) ([]MarkupSpan, string, bool) {
	if !strings.HasPrefix(source, "***") || strings.Contains(source[3:], "***") {
		return nil, source, false
	}
	body := source[3:]
	boldEnd := strings.Index(body, "**")
	if boldEnd < 0 {
		return nil, source, false
	}
	rest := body[boldEnd+2:]
	italicEnd := strings.IndexByte(rest, '*')
	if italicEnd < 0 {
		return nil, source, false
	}
	bold := style
	bold.Bold = true
	bold.Italic = true
	bold.Text = body[:boldEnd]
	italic := style
	italic.Italic = true
	italic.Text = rest[:italicEnd]
	return compactSpans([]MarkupSpan{bold, italic}), rest[italicEnd+1:], true
}

func parseLinkedSpan(source string, style MarkupSpan) (MarkupSpan, string, bool) {
	image := strings.HasPrefix(source, "![")
	if !image && !strings.HasPrefix(source, "[") {
		return MarkupSpan{}, source, false
	}
	labelStart := 1
	if image {
		labelStart = 2
	}
	closeLabel := strings.Index(source[labelStart:], "](")
	if closeLabel < 0 {
		return MarkupSpan{}, source, false
	}
	closeLabel += labelStart
	closeTarget := strings.IndexByte(source[closeLabel+2:], ')')
	if closeTarget < 0 {
		return MarkupSpan{}, source, false
	}
	closeTarget += closeLabel + 2
	style.Text = source[labelStart:closeLabel]
	style.Link = source[closeLabel+2 : closeTarget]
	style.Kind = classifyLink(style.Link, image)
	return style, source[closeTarget+1:], true
}

func emphasis(source string, style MarkupSpan) (open, close string, next MarkupSpan, ok bool) {
	next = style
	switch {
	case strings.HasPrefix(source, "***"):
		next.Bold = true
		return "***", "***", next, true
	case strings.HasPrefix(source, "**"):
		next.Bold = true
		return "**", "**", next, true
	case strings.HasPrefix(source, "*"):
		next.Italic = true
		return "*", "*", next, true
	case strings.HasPrefix(source, "__"):
		next.Underline = true
		return "__", "__", next, true
	case strings.HasPrefix(source, "~~"):
		next.Strike = true
		return "~~", "~~", next, true
	}
	return "", "", next, false
}

func classifyLink(target string, image bool) MarkupKind {
	if image {
		return MarkupImage
	}
	ext := strings.ToLower(filepath.Ext(target))
	if strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") || strings.HasPrefix(target, "/") {
		if ext != "" {
			return MarkupFile
		}
	}
	return MarkupLink
}

func nextMarkupStart(s string) int {
	next := -1
	for _, marker := range []string{"![", "[", "***", "**", "*", "__", "~~"} {
		if i := strings.Index(s, marker); i >= 0 && (next < 0 || i < next) {
			next = i
		}
	}
	return next
}

func compactSpans(spans []MarkupSpan) []MarkupSpan {
	out := spans[:0]
	for _, span := range spans {
		if span.Text == "" {
			continue
		}
		if len(out) > 0 && sameSpanKind(out[len(out)-1], span) {
			out[len(out)-1].Text += span.Text
			continue
		}
		out = append(out, span)
	}
	return out
}

func splitMarkupLines(spans []MarkupSpan) [][]MarkupSpan {
	lines := [][]MarkupSpan{{}}
	for _, span := range spans {
		parts := strings.Split(span.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				lines = append(lines, nil)
			}
			lines[len(lines)-1] = appendSpanText(lines[len(lines)-1], span, part)
		}
	}
	return lines
}

func appendSpanText(spans []MarkupSpan, tmpl MarkupSpan, s string) []MarkupSpan {
	if s == "" {
		return spans
	}
	tmpl.Text = s
	if len(spans) > 0 && sameSpanKind(spans[len(spans)-1], tmpl) {
		spans[len(spans)-1].Text += s
		return spans
	}
	return append(spans, tmpl)
}

func spansWidth(spans []MarkupSpan) int {
	width := 0
	for _, span := range spans {
		width += text.Width(span.Text)
	}
	return width
}

func mergeStyle(base, overlay screen.Style) screen.Style {
	if overlay.Fg.Set() {
		base.Fg = overlay.Fg
	}
	if overlay.Bg.Set() {
		base.Bg = overlay.Bg
	}
	base.Attrs |= overlay.Attrs
	return base
}

func sameSpanKind(a, b MarkupSpan) bool {
	return a.Kind == b.Kind &&
		a.Bold == b.Bold &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Strike == b.Strike &&
		a.Link == b.Link
}
