// Package picker holds the pure logic behind the emoji / sticker picker:
// filtering the unicode emoji table, and building the strings that get inserted
// into the composer for a chosen emoji or sticker.
//
// It is deliberately free of any UI or Discord-library types so the ordering,
// filtering, and "fake-nitro" URL construction can be table-tested in isolation.
// The ui layer bridges store catalog data into these primitives.
//
// # Fake nitro
//
// Discord only lets an account send an animated or other-guild custom emoji
// inline when it has Nitro. Without Nitro, tuicord falls back to pasting the
// emoji's CDN URL as message content; PLAN #3's media classifier recognizes
// those URLs and renders them back as the real emoji/sticker for anyone reading
// in tuicord. [EmojiCDNURL] and [StickerCDNURL] build those URLs, and
// [EmojiInsert]/[StickerInsert] pick between the native mention form and the
// fake-nitro fallback.
package picker

import (
	"net/url"
	"strconv"
	"strings"

	"awesomeProject/internal/markup"
)

// Emoji is one entry in the static unicode emoji table.
type Emoji struct {
	// Char is the literal unicode emoji inserted into the composer.
	Char string
	// Name is the primary shortcode-style name (e.g. "grinning").
	Name string
	// Keywords are extra search terms that match this emoji.
	Keywords []string
}

// Unicode returns a copy of the built-in unicode emoji table.
func Unicode() []Emoji {
	out := make([]Emoji, len(unicodeEmoji))
	copy(out, unicodeEmoji)
	return out
}

// FilterEmoji returns up to limit emoji from the unicode table matching query.
//
// Matching is case-insensitive against the name and keywords. An empty query
// returns the head of the table. Results are ordered so that name-prefix matches
// come first, then name-substring matches, then keyword matches, preserving the
// table's order within each tier. A limit <= 0 returns every match.
func FilterEmoji(query string, limit int) []Emoji {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return headEmoji(limit)
	}
	var prefix, nameSub, keyword []Emoji
	for _, e := range unicodeEmoji {
		name := strings.ToLower(e.Name)
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, e)
		case strings.Contains(name, q):
			nameSub = append(nameSub, e)
		case matchesKeyword(e.Keywords, q):
			keyword = append(keyword, e)
		}
	}
	out := append(prefix, nameSub...)
	out = append(out, keyword...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func headEmoji(limit int) []Emoji {
	n := len(unicodeEmoji)
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]Emoji, n)
	copy(out, unicodeEmoji[:n])
	return out
}

func matchesKeyword(keywords []string, q string) bool {
	for _, k := range keywords {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	return false
}

// EmojiMention returns the Discord inline form for a custom emoji,
// "<:name:id>" (or "<a:name:id>" when animated). This is what renders natively
// for accounts that may use the emoji.
func EmojiMention(id uint64, name string, animated bool) string {
	prefix := "<:"
	if animated {
		prefix = "<a:"
	}
	return prefix + name + ":" + strconv.FormatUint(id, 10) + ">"
}

// EmojiCDNURL returns the fake-nitro CDN URL for a custom emoji. The extension
// follows the animated flag and the name rides along as a query parameter so the
// classifier and readers can recover it.
func EmojiCDNURL(id uint64, name string, animated bool) string {
	ext := "png"
	if animated {
		ext = "gif"
	}
	u := "https://cdn.discordapp.com/emojis/" + strconv.FormatUint(id, 10) + "." + ext + "?size=48"
	if name != "" {
		u += "&name=" + url.QueryEscape(name)
	}
	return u
}

// StickerCDNURL returns the fake-nitro CDN URL for a sticker. Stickers are
// served as PNG on the media origin the classifier recognizes.
func StickerCDNURL(id uint64) string {
	return "https://media.discordapp.net/stickers/" + strconv.FormatUint(id, 10) + ".png"
}

// CanUseEmoji reports whether the account may send the emoji inline as a native
// mention. Static emoji from a guild the user is in are free; animated or
// other-guild emoji require Nitro.
func CanUseEmoji(sameGuild, animated, nitro bool) bool {
	return nitro || (sameGuild && !animated)
}

// EmojiInsert returns the string to insert into the composer for a chosen custom
// emoji, and whether an insert is possible at all. It prefers the native mention
// form when the account may use the emoji; otherwise it falls back to the
// marked fake-nitro CDN link when fakeNitro is enabled. When neither path is
// available (no Nitro and fake-nitro disabled) it returns ("", false).
func EmojiInsert(id uint64, name string, animated, sameGuild, nitro, fakeNitro bool) (string, bool) {
	if CanUseEmoji(sameGuild, animated, nitro) {
		return EmojiMention(id, name, animated), true
	}
	if fakeNitro {
		return markup.FakeEmojiLink(name, EmojiCDNURL(id, name, animated)), true
	}
	return "", false
}

// StickerInsert returns the string to insert for a chosen sticker, and whether
// an insert is possible. tuicord sends stickers as marked CDN links (the
// fake-nitro path), which render inline for tuicord readers regardless of
// Nitro. When fakeNitro is disabled it returns ("", false), since native
// sticker sends are not yet wired.
func StickerInsert(id uint64, name string, fakeNitro bool) (string, bool) {
	if !fakeNitro {
		return "", false
	}
	return markup.FakeStickerLink(name, StickerCDNURL(id)), true
}
