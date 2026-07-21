package ui

// SetStickyAnchor toggles message-identity viewport anchoring (the
// display.sticky_anchor setting). When disabled, a scrolled viewport keeps only
// its bottom-relative offset, so height changes above it shift the view.
func (w *ChatView) SetStickyAnchor(enabled bool) {
	if w == nil {
		return
	}
	w.stickyAnchor = enabled
	if !enabled {
		w.anchorSet = false
	}
}

// captureAnchor records which message line sits at the top of the viewport so
// the next draw can re-anchor to it after a relayout. Lines without a message
// (frame-only rows) are skipped; the row distance to the first keyed line is
// kept in anchorDelta so restoring reproduces the exact viewport top.
func (w *ChatView) captureAnchor(lines []chatLine, start int) {
	w.anchorSet = false
	if !w.stickyAnchor {
		return
	}
	for i := start; i < len(lines); i++ {
		key := chatLineAnchorKey(lines[i])
		if key == "" {
			continue
		}
		first := i
		for first > 0 && chatLineAnchorKey(lines[first-1]) == key {
			first--
		}
		w.anchorKey = key
		w.anchorIntra = i - first
		w.anchorDelta = i - start
		w.anchorOffset = w.bottomScroll.Offset()
		w.anchorSet = true
		return
	}
}

// anchorLineIndex locates the line index the anchor points at in a freshly
// rendered transcript: the anchor message's block start plus the remembered
// intra-message row, clamped to the block so a message that shrank still
// anchors inside itself.
func anchorLineIndex(lines []chatLine, key string, intra int) (int, bool) {
	first := -1
	last := -1
	for i := range lines {
		if chatLineAnchorKey(lines[i]) != key {
			if first >= 0 {
				break
			}
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 {
		return 0, false
	}
	return first + min(intra, last-first), true
}

// chatLineAnchorKey identifies the message a rendered line belongs to, or ""
// for lines that carry no message.
func chatLineAnchorKey(line chatLine) string {
	if line.message.ID == 0 && line.message.Nonce == "" {
		return ""
	}
	return messagePlacementPrefix(line.message)
}
