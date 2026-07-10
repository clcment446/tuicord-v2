package picker

import (
	"strings"
	"testing"

	"awesomeProject/internal/media"
)

func TestFilterEmojiEmptyReturnsHead(t *testing.T) {
	got := FilterEmoji("", 5)
	if len(got) != 5 {
		t.Fatalf("empty query limit 5 = %d entries, want 5", len(got))
	}
	if got[0].Name != unicodeEmoji[0].Name {
		t.Fatalf("head[0] = %+v, want table head %+v", got[0], unicodeEmoji[0])
	}
}

func TestFilterEmojiEmptyUnlimited(t *testing.T) {
	if got := FilterEmoji("  ", 0); len(got) != len(unicodeEmoji) {
		t.Fatalf("empty unlimited = %d, want %d", len(got), len(unicodeEmoji))
	}
}

func TestFilterEmojiNameMatch(t *testing.T) {
	got := FilterEmoji("fire", 0)
	if len(got) == 0 || got[0].Name != "fire" {
		t.Fatalf("filter 'fire' = %+v, want fire first", got)
	}
}

func TestFilterEmojiCaseInsensitive(t *testing.T) {
	lower := FilterEmoji("heart", 0)
	upper := FilterEmoji("HEART", 0)
	if len(lower) != len(upper) || len(lower) == 0 {
		t.Fatalf("case sensitivity: lower=%d upper=%d", len(lower), len(upper))
	}
}

func TestFilterEmojiPrefixBeatsSubstring(t *testing.T) {
	// "heart" is a prefix of "heart" and "heart_eyes"; it is also a substring of
	// "sparkling_heart". Prefix matches must come before substring matches.
	got := FilterEmoji("heart", 0)
	firstSub := -1
	for i, e := range got {
		if !strings.HasPrefix(strings.ToLower(e.Name), "heart") {
			firstSub = i
			break
		}
	}
	if firstSub == 0 {
		t.Fatalf("first result is a non-prefix match: %+v", got[0])
	}
}

func TestFilterEmojiKeywordMatch(t *testing.T) {
	// "lol" is not in any name but is a keyword for joy/rofl/laughing.
	got := FilterEmoji("lol", 0)
	if len(got) == 0 {
		t.Fatal("keyword 'lol' matched nothing")
	}
	for _, e := range got {
		if strings.Contains(strings.ToLower(e.Name), "lol") {
			continue
		}
		if !matchesKeyword(e.Keywords, "lol") {
			t.Fatalf("result %+v matches neither name nor keyword 'lol'", e)
		}
	}
}

func TestFilterEmojiLimit(t *testing.T) {
	if got := FilterEmoji("a", 3); len(got) > 3 {
		t.Fatalf("limit 3 returned %d", len(got))
	}
}

func TestFilterEmojiNoMatch(t *testing.T) {
	if got := FilterEmoji("zzzzznotanemoji", 0); len(got) != 0 {
		t.Fatalf("nonsense query matched %d", len(got))
	}
}

func TestUnicodeIsCopy(t *testing.T) {
	a := Unicode()
	if len(a) == 0 {
		t.Fatal("Unicode() empty")
	}
	a[0].Name = "mutated"
	if unicodeEmoji[0].Name == "mutated" {
		t.Fatal("Unicode() aliased the package table")
	}
}

func TestEmojiMention(t *testing.T) {
	if got := EmojiMention(123, "blob", false); got != "<:blob:123>" {
		t.Fatalf("static mention = %q", got)
	}
	if got := EmojiMention(456, "wave", true); got != "<a:wave:456>" {
		t.Fatalf("animated mention = %q", got)
	}
}

func TestEmojiCDNURLRoundTrip(t *testing.T) {
	static := EmojiCDNURL(111, "party", false)
	if !strings.HasSuffix(strings.SplitN(static, "?", 2)[0], ".png") {
		t.Fatalf("static emoji url not png: %q", static)
	}
	if media.ClassifyURL(static) != media.ClassEmoji {
		t.Fatalf("static emoji url did not classify as emoji: %q", static)
	}
	animated := EmojiCDNURL(222, "spin", true)
	if !strings.Contains(animated, ".gif") {
		t.Fatalf("animated emoji url not gif: %q", animated)
	}
	if media.ClassifyURL(animated) != media.ClassEmoji {
		t.Fatalf("animated emoji url did not classify as emoji: %q", animated)
	}
	if !strings.Contains(static, "name=party") {
		t.Fatalf("emoji url missing name param: %q", static)
	}
}

func TestEmojiCDNURLEscapesName(t *testing.T) {
	got := EmojiCDNURL(1, "my emoji", false)
	if strings.Contains(got, "my emoji") {
		t.Fatalf("name not escaped: %q", got)
	}
	if !strings.Contains(got, "my+emoji") && !strings.Contains(got, "my%20emoji") {
		t.Fatalf("escaped name missing: %q", got)
	}
}

func TestStickerCDNURLRoundTrip(t *testing.T) {
	u := StickerCDNURL(9876)
	if media.ClassifyURL(u) != media.ClassSticker {
		t.Fatalf("sticker url did not classify as sticker: %q", u)
	}
}

func TestCanUseEmoji(t *testing.T) {
	cases := []struct {
		sameGuild, animated, nitro, want bool
	}{
		{true, false, false, true},   // own static: free
		{true, true, false, false},   // own animated: needs nitro
		{false, false, false, false}, // other-guild static: needs nitro
		{false, true, true, true},    // nitro: anything
		{true, true, true, true},     // nitro own animated
	}
	for _, c := range cases {
		if got := CanUseEmoji(c.sameGuild, c.animated, c.nitro); got != c.want {
			t.Errorf("CanUseEmoji(%v,%v,%v)=%v want %v", c.sameGuild, c.animated, c.nitro, got, c.want)
		}
	}
}

func TestEmojiInsert(t *testing.T) {
	// Usable → native mention.
	if got, ok := EmojiInsert(5, "ok", false, true, false, true); !ok || got != "<:ok:5>" {
		t.Fatalf("own static insert = %q,%v want native mention", got, ok)
	}
	// Not usable, fake nitro on → CDN URL.
	got, ok := EmojiInsert(7, "spin", true, false, false, true)
	if !ok || media.ClassifyURL(got) != media.ClassEmoji {
		t.Fatalf("fake-nitro insert = %q,%v want emoji url", got, ok)
	}
	// Not usable, fake nitro off → cannot insert.
	if _, ok := EmojiInsert(7, "spin", true, false, false, false); ok {
		t.Fatal("expected no insert when unusable and fake-nitro disabled")
	}
}

func TestStickerInsert(t *testing.T) {
	got, ok := StickerInsert(42, true)
	if !ok || media.ClassifyURL(got) != media.ClassSticker {
		t.Fatalf("sticker insert = %q,%v want sticker url", got, ok)
	}
	if _, ok := StickerInsert(42, false); ok {
		t.Fatal("expected no sticker insert when fake-nitro disabled")
	}
}
