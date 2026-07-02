package text

import "testing"

func TestWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"cjk", "テスト", 6},
		{"chinese", "测试", 4},
		{"hangul", "한글", 4},
		{"fullwidth ascii", "ＡＢ", 4},
		{"ideographic space U+3000", "　", 2},
		{"emoji", "🎉", 2},
		{"zwj family single glyph", "👨‍👩‍👧", 2},
		{"rainbow flag zwj+vs16", "🏳️‍🌈", 2},
		{"flag pair", "🇯🇵", 2},
		{"skin tone modifier cluster", "👍🏽", 2},
		{"keycap sequence", "1️⃣", 2},
		{"vs16 promotes heart", "❤️", 2},
		{"bare heart stays narrow", "❤", 1},
		{"vs16 promotes bmp storm", "⛈️", 2},
		{"vs15 demotes hourglass", "⌛︎", 1},
		{"mountain needs presentation bump", "🏔", 2},
		{"combining mark folds in", "é", 1},
		{"precomposed same width", "é", 1},
		{"combining in word", "café", 4},
		{"zwj alone", "‍", 0},
		{"vs16 alone", "️", 0},
		{"control runes", "a\x00\x1b b", 3},
		{"c1 control", "ab", 2},
		{"mixed discord username", "mira🎉テスト", 12},
		{"emoji between ascii", "a🎉b", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Width(tt.in); got != tt.want {
				t.Errorf("Width(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClusterWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"single ascii", "a", 1},
		{"wide cjk", "テ", 2},
		{"zwj family", "👨‍👩‍👧", 2},
		{"vs16 promotion", "❤️", 2},
		{"vs15 demotion", "⌛︎", 1},
		{"flag pair", "🇺🇸", 2},
		{"lone combining mark", "́", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClusterWidth(tt.in); got != tt.want {
				t.Errorf("ClusterWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClustersIteration(t *testing.T) {
	// 5 runes, 3 clusters: a, family (3 emoji + 2 ZWJ), b
	s := "a👨‍👩‍👧b"
	var got []Cluster
	for c := range Clusters(s) {
		got = append(got, c)
	}
	if len(got) != 3 {
		t.Fatalf("got %d clusters, want 3: %+v", len(got), got)
	}
	if got[0].Text != "a" || got[0].Offset != 0 || got[0].Width != 1 {
		t.Errorf("cluster 0 = %+v", got[0])
	}
	if got[1].Text != "👨‍👩‍👧" || got[1].Offset != 1 || got[1].Width != 2 {
		t.Errorf("cluster 1 = %+v", got[1])
	}
	if got[2].Text != "b" || got[2].Width != 1 {
		t.Errorf("cluster 2 = %+v", got[2])
	}
	if got[2].Offset != 1+len("👨‍👩‍👧") {
		t.Errorf("cluster 2 offset = %d", got[2].Offset)
	}
}

func TestBoundaries(t *testing.T) {
	s := "a👨‍👩‍👧b"
	fam := len("👨‍👩‍👧")

	if got := NextBoundary(s, 0); got != 1 {
		t.Errorf("NextBoundary(0) = %d, want 1", got)
	}
	if got := NextBoundary(s, 1); got != 1+fam {
		t.Errorf("NextBoundary(1) = %d, want %d", got, 1+fam)
	}
	if got := NextBoundary(s, len(s)); got != len(s) {
		t.Errorf("NextBoundary(len) = %d, want %d", got, len(s))
	}
	if got := PrevBoundary(s, len(s)); got != len(s)-1 {
		t.Errorf("PrevBoundary(len) = %d, want %d", got, len(s)-1)
	}
	// Deleting backwards from after the family removes the WHOLE family.
	if got := PrevBoundary(s, 1+fam); got != 1 {
		t.Errorf("PrevBoundary(after family) = %d, want 1", got)
	}
	if got := PrevBoundary(s, 0); got != 0 {
		t.Errorf("PrevBoundary(0) = %d, want 0", got)
	}
	// Clamping.
	if got := NextBoundary(s, -5); got != 1 {
		t.Errorf("NextBoundary(-5) = %d, want 1", got)
	}
	if got := PrevBoundary(s, len(s)+10); got != len(s)-1 {
		t.Errorf("PrevBoundary(overshoot) = %d, want %d", got, len(s)-1)
	}
}
