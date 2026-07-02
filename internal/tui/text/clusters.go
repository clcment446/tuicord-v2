package text

import (
	"iter"

	"github.com/rivo/uniseg"
)

// Cluster is one user-perceived character: a grapheme cluster with its
// position in the source string and its display width in cells.
type Cluster struct {
	// Text is the cluster's bytes, possibly several runes (base + combining
	// marks, ZWJ sequences, flag pairs).
	Text string
	// Offset is the byte offset of the cluster in the source string.
	Offset int
	// Width is the display width in cells (0, 1, or 2).
	Width int
}

// Clusters iterates over the grapheme clusters of s in order. This is the
// only correct unit for truncation, wrapping, and cursor movement — slicing
// by bytes splits UTF-8, slicing by runes orphans combining marks and breaks
// ZWJ emoji.
func Clusters(s string) iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		offset := 0
		state := -1
		for len(s) > 0 {
			var cluster string
			cluster, s, _, state = uniseg.FirstGraphemeClusterInString(s, state)
			c := Cluster{Text: cluster, Offset: offset, Width: ClusterWidth(cluster)}
			if !yield(c) {
				return
			}
			offset += len(cluster)
		}
	}
}

// NextBoundary returns the byte offset of the next grapheme boundary after
// offset i, clamped to len(s). Use it to move a cursor right by one
// user-perceived character.
func NextBoundary(s string, i int) int {
	if i < 0 {
		i = 0
	}
	if i >= len(s) {
		return len(s)
	}
	cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
	return i + len(cluster)
}

// PrevBoundary returns the byte offset of the previous grapheme boundary
// before offset i, clamped to 0. Use it to move a cursor left by one
// user-perceived character or to delete backwards without orphaning marks.
func PrevBoundary(s string, i int) int {
	if i > len(s) {
		i = len(s)
	}
	if i <= 0 {
		return 0
	}
	prev := 0
	for c := range Clusters(s[:i]) {
		prev = c.Offset
	}
	return prev
}
