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

// Fold and unfold toggles pin the toggled control's line to the screen row it
// occupied when activated, exactly like Discord. The message-level anchor
// cannot do this: it maps line positions by index within the message, and a
// fold changes every index after the control, so repeated fold/unfold cycles
// would creep the viewport.
const (
	pendingAnchorNone uint8 = iota
	pendingAnchorHeader
	pendingAnchorControl
)

// anchorHeaderToggle pins a markdown-header fold control (by header key) at
// the given visible row for the next draw.
func (w *ChatView) anchorHeaderToggle(key string, row int) {
	if !w.stickyAnchor || key == "" || row < 0 {
		return
	}
	w.pendingAnchorKind = pendingAnchorHeader
	w.pendingAnchorKey = key
	w.pendingAnchorRow = row
}

// anchorControlToggle pins an expandable component control at the visible row
// it currently occupies. The control is located by key because component
// activations arrive without coordinates (shortcuts, keyboard traversal).
func (w *ChatView) anchorControlToggle(controlKey string) {
	if !w.stickyAnchor || controlKey == "" {
		return
	}
	for row := range w.visibleLines {
		if pendingAnchorMatches(w.visibleLines[row], pendingAnchorControl, controlKey) {
			w.pendingAnchorKind = pendingAnchorControl
			w.pendingAnchorKey = controlKey
			w.pendingAnchorRow = row
			return
		}
	}
}

// applyPendingAnchor consumes a pending fold/unfold pin, scrolling so the
// toggled control sits at its remembered row. It reports whether it took
// effect, in which case the generic anchor restore must not also run.
func (w *ChatView) applyPendingAnchor(lines []chatLine, height int) bool {
	kind := w.pendingAnchorKind
	if kind == pendingAnchorNone {
		return false
	}
	w.pendingAnchorKind = pendingAnchorNone
	for i := range lines {
		if pendingAnchorMatches(lines[i], kind, w.pendingAnchorKey) {
			w.bottomScroll.SetOffset(len(lines) - height - (i - w.pendingAnchorRow))
			return true
		}
	}
	return false
}

func pendingAnchorMatches(line chatLine, kind uint8, key string) bool {
	switch kind {
	case pendingAnchorHeader:
		return line.header != nil && line.header.key == key
	case pendingAnchorControl:
		for _, hit := range line.actions {
			if hit.action.controlKey() == key {
				return true
			}
		}
	}
	return false
}
