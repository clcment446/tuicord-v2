package text

import (
	"reflect"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		max   int
		tail  string
		want  string
		width int // max acceptable resulting width
	}{
		{"fits untouched", "hello", 10, Ellipsis, "hello", 5},
		{"exact fit untouched", "hello", 5, Ellipsis, "hello", 5},
		{"ascii cut", "hello world", 8, Ellipsis, "hello w…", 8},
		{"zero budget", "hello", 0, Ellipsis, "", 0},
		{"no tail", "hello", 3, "", "hel", 3},
		{"cjk never split mid glyph", "テスト", 4, Ellipsis, "テ…", 4},
		{"cjk odd budget", "テスト", 5, Ellipsis, "テス…", 5},
		{"family kept whole or dropped", "ab👨‍👩‍👧cd", 4, Ellipsis, "ab…", 4},
		{"emoji fits with tail", "ab👨‍👩‍👧cd", 5, Ellipsis, "ab👨‍👩‍👧…", 5},
		{"tail wider than budget", "hello", 1, "……", "……", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.in, tt.max, tt.tail)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d, %q) = %q, want %q", tt.in, tt.max, tt.tail, got, tt.want)
			}
		})
	}
}

func TestPad(t *testing.T) {
	if got := PadRight("ab", 5); got != "ab   " {
		t.Errorf("PadRight = %q", got)
	}
	if got := PadLeft("ab", 5); got != "   ab" {
		t.Errorf("PadLeft = %q", got)
	}
	// CJK measured by cells, not runes: テスト is 6 cells.
	if got := PadRight("テスト", 8); got != "テスト  " {
		t.Errorf("PadRight cjk = %q", got)
	}
	// Overlong input is truncated to the exact column width.
	if got := Width(PadRight("テスト", 5)); got != 5 {
		t.Errorf("PadRight overlong width = %d, want 5", got)
	}
	if got := Width(PadRight("mira🎉テスト", 7)); got != 7 {
		t.Errorf("PadRight username width = %d, want 7", got)
	}
	if got := PadRight("x", 0); got != "" {
		t.Errorf("PadRight zero = %q", got)
	}
}

func TestExpandTabs(t *testing.T) {
	if got := ExpandTabs("a\tb", 4); got != "a   b" {
		t.Errorf("ExpandTabs = %q", got)
	}
	if got := ExpandTabs("\tb", 4); got != "    b" {
		t.Errorf("ExpandTabs leading = %q", got)
	}
	// Tab stop position accounts for cell width: テ is 2 cells wide.
	if got := ExpandTabs("テ\tb", 4); got != "テ  b" {
		t.Errorf("ExpandTabs after wide = %q", got)
	}
	// Newline resets the column.
	if got := ExpandTabs("ab\n\tc", 4); got != "ab\n    c" {
		t.Errorf("ExpandTabs newline reset = %q", got)
	}
	if got := ExpandTabs("no tabs", 4); got != "no tabs" {
		t.Errorf("ExpandTabs passthrough = %q", got)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"fits", "hello", 10, []string{"hello"}},
		{"word wrap", "hello brave world", 11, []string{"hello brave", "world"}},
		{"prefers space", "aaa bbb", 5, []string{"aaa", "bbb"}},
		{"hard break long run", "abcdefgh", 3, []string{"abc", "def", "gh"}},
		{"cjk hard break by cells", "テストです", 4, []string{"テス", "トで", "す"}},
		{"respects newlines", "ab\ncd", 10, []string{"ab", "cd"}},
		{"empty line kept", "ab\n\ncd", 10, []string{"ab", "", "cd"}},
		{"zero width returns as is", "hello", 0, []string{"hello"}},
		{"emoji not split", "ab🎉", 3, []string{"ab", "🎉"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Wrap(tt.in, tt.width)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Wrap(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}
		})
	}

	// Every produced line must actually fit (except unsplittable clusters).
	for _, line := range Wrap("mira: did you see the ⛈️ today? テスト 👨‍👩‍👧 yes!", 10) {
		if w := Width(line); w > 10 {
			t.Errorf("wrapped line %q measures %d cells, budget 10", line, w)
		}
	}
}
