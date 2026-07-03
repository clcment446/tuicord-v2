package markup

import "testing"

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
		{Kind: Kind_Mention, Text: "@alice"},
		{Kind: Kind_Text, Text: " in "},
		{Kind: Kind_ChannelMention, Text: "#general"},
	}
	assertSpans(t, spans, want)
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
		{Kind: Kind_Emoji, Text: ":wave:"},
		{Kind: Kind_Text, Text: " "},
		{Kind: Kind_Emoji, Text: ":spin:"},
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

func TestParseMalformedDegradesToText(t *testing.T) {
	// Unterminated constructs stay literal.
	for _, in := range []string{"**bold", "`code", "[label](url", "<@notanumber>", "<broken"} {
		spans := Parse(in, Resolver{})
		if len(spans) != 1 || spans[0].Kind != Kind_Text || spans[0].Text != in {
			t.Errorf("Parse(%q) = %+v, want single text span", in, spans)
		}
	}
}

func assertSpans(t *testing.T, got, want []Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d spans, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
