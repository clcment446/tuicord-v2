package widget

import (
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestMarkupParsesLinksAndBold(t *testing.T) {
	m := NewMarkup("read [docs](https://example.test) and ***ship it***")
	spans := m.Spans()
	want := []MarkupSpan{
		{Text: "read "},
		{Text: "docs", Kind: MarkupLink, Link: "https://example.test"},
		{Text: " and "},
		{Text: "ship it", Bold: true},
	}
	if len(spans) != len(want) {
		t.Fatalf("len(spans) = %d, want %d: %#v", len(spans), len(want), spans)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Fatalf("span %d = %#v, want %#v", i, spans[i], want[i])
		}
	}
}

func TestMarkupUnclosedBoldRunsToEnd(t *testing.T) {
	m := NewMarkup("***bold")
	spans := m.Spans()
	if len(spans) != 1 || spans[0] != (MarkupSpan{Text: "bold", Bold: true}) {
		t.Fatalf("spans = %#v", spans)
	}
}

func TestMarkupDrawsStyledTextWithoutDelimiters(t *testing.T) {
	m := NewMarkup("[go]() ***now***")
	buf := screen.NewBuffer(6, 1)
	m.Draw(buf.Clip(buf.Bounds()))

	if got, want := bufferRow(buf, 0), "go now"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
	if got := buf.Cell(0, 0).Style.Attrs; got&screen.Underline == 0 {
		t.Fatalf("link attrs = %v, want underline", got)
	}
	if got := buf.Cell(3, 0).Style.Attrs; got&screen.Bold == 0 {
		t.Fatalf("bold attrs = %v, want bold", got)
	}
}

func TestMarkupRequestedSyntax(t *testing.T) {
	m := NewMarkup("[links]() ***bold** italic* __underlined__~~strikethrough~~ ![images](./pic.png) [files](./attachements/tmp.log)")
	spans := m.Spans()
	assertSpan(t, spans, "links", func(s MarkupSpan) bool {
		return s.Kind == MarkupLink && s.Link == ""
	})
	assertSpan(t, spans, "bold", func(s MarkupSpan) bool {
		return s.Bold && s.Italic
	})
	assertSpan(t, spans, " italic", func(s MarkupSpan) bool {
		return s.Italic && !s.Bold
	})
	assertSpan(t, spans, "underlined", func(s MarkupSpan) bool {
		return s.Underline
	})
	assertSpan(t, spans, "strikethrough", func(s MarkupSpan) bool {
		return s.Strike
	})
	assertSpan(t, spans, "images", func(s MarkupSpan) bool {
		return s.Kind == MarkupImage && s.Link == "./pic.png"
	})
	assertSpan(t, spans, "files", func(s MarkupSpan) bool {
		return s.Kind == MarkupFile && s.Link == "./attachements/tmp.log"
	})
}

func assertSpan(t *testing.T, spans []MarkupSpan, text string, ok func(MarkupSpan) bool) {
	t.Helper()
	for _, span := range spans {
		if span.Text == text && ok(span) {
			return
		}
	}
	t.Fatalf("span %q not found in %#v", text, spans)
}
