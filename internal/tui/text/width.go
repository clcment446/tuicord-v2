// Package text is the measurement foundation of the TUI library.
//
// Everything drawn to the terminal is measured here, in display cells, by
// grapheme cluster. The three counts — len(s) bytes, rune count, and cell
// width — are different numbers; only cell width is valid for layout, and
// only grapheme clusters are valid units for truncation and cursor movement.
//
// All functions operate on plain text: strip ANSI escapes and markup before
// measuring. Tabs and newlines are layout concerns, not glyphs — expand tabs
// with ExpandTabs and split lines before measuring; stray control runes
// measure as zero cells.
package text

import (
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

const (
	// VS15 (U+FE0E) requests text presentation, forcing a 2-cell emoji base
	// down to 1 cell. VS16 (U+FE0F) requests emoji presentation, promoting a
	// 1-cell base (e.g. ❤ U+2764) to 2 cells. Both are width 0 themselves.
	VS15 = '︎'
	VS16 = '️'

	zwj = '‍'
)

// Width returns the display width of s in terminal cells.
//
// s must be plain text (no ANSI escapes, no markup). Control runes,
// combining marks, variation selectors, and ZWJ measure zero; grapheme
// clusters are measured as units so ZWJ sequences, flags, keycaps, and
// skin-tone modifiers count once.
func Width(s string) int {
	w := 0
	state := -1
	for len(s) > 0 {
		var cluster string
		cluster, s, _, state = uniseg.FirstGraphemeClusterInString(s, state)
		w += ClusterWidth(cluster)
	}
	return w
}

// ClusterWidth returns the display width in cells of a single grapheme
// cluster. The base width comes from the first rune; a VS16 anywhere in the
// cluster promotes it to 2, a VS15 demotes it to 1.
func ClusterWidth(cluster string) int {
	first := true
	w := 0
	for _, r := range cluster {
		if first {
			w = baseRuneWidth(r)
			first = false
			continue
		}
		switch r {
		case VS16:
			if w == 1 {
				w = 2
			}
		case VS15:
			if w == 2 {
				w = 1
			}
		}
	}
	return w
}

// baseRuneWidth returns the cell width of r as a cluster base, correcting
// go-runewidth where modern terminals disagree with EastAsianWidth (emoji
// presentation blocks and regional indicators draw 2 cells wide).
func baseRuneWidth(r rune) int {
	switch {
	case r == utf8.RuneError:
		return 1
	case r < 0x20, r >= 0x7F && r < 0xA0: // C0 and C1 controls (incl. \t, \n)
		return 0
	case isEmojiPresentation(r):
		return 2
	}
	return runewidth.RuneWidth(r)
}

// isEmojiPresentation reports whether r defaults to emoji presentation —
// glyphs terminals draw double-wide regardless of EastAsianWidth. Condensed
// from the Unicode 15.1 Emoji_Presentation property to the ranges that occur
// in user content; ported from the tuicord renderer, plus regional
// indicators (flag halves), which it missed.
func isEmojiPresentation(r rune) bool {
	switch {
	case r >= 0x1F1E6 && r <= 0x1F1FF: // Regional Indicators (flag pairs)
		return true
	case r >= 0x1F300 && r <= 0x1F64F: // Misc Symbols & Pictographs + Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport & Map
		return true
	case r >= 0x1F700 && r <= 0x1F77F: // Alchemical
		return true
	case r >= 0x1F780 && r <= 0x1F7FF: // Geometric Shapes Ext
		return true
	case r >= 0x1F800 && r <= 0x1F8FF: // Supplemental Arrows-C
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols & Pictographs
		return true
	case r >= 0x1FA00 && r <= 0x1FAFF: // Chess / Symbols & Pictographs Ext-A
		return true
	case r >= 0x1FB00 && r <= 0x1FBFF: // Symbols for Legacy Computing
		return true
	}
	return false
}
