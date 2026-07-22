package ui

import (
	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func (w *ChatView) selectionContainsLine(line int) bool {
	if w == nil || !w.selectionActive || w.selectionStart < 0 || w.focusStopIndex < 0 {
		return false
	}
	start, end := w.selectionStart, w.focusStopIndex
	if start > end {
		start, end = end, start
	}
	for i := start; i <= end && i < len(w.focusStops); i++ {
		range_, ok := w.focusRanges[w.focusStops[i].messageKey]
		if ok && line >= range_.start && line < range_.end {
			return true
		}
	}
	return false
}

func (w *ChatView) focusedStopAt(line int) (chatFocusStop, bool) {
	if w == nil || !w.keyboardFocused || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return chatFocusStop{}, false
	}
	stop := w.focusStops[w.focusStopIndex]
	return stop, stop.line == line
}

func (w *ChatView) focusedHighlightAt(line int) (chatFocusStop, bool, bool) {
	stop, exact := w.focusedStopAt(line)
	if !w.highlightFocusBlock || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return stop, exact, false
	}
	stop = w.focusStops[w.focusStopIndex]
	if line < stop.line {
		return chatFocusStop{}, false, false
	}
	end := w.focusRanges[stop.messageKey].end
	for i := w.focusStopIndex + 1; i < len(w.focusStops); i++ {
		next := w.focusStops[i]
		if next.messageKey != stop.messageKey {
			break
		}
		if next.line > stop.line {
			end = min(end, next.line)
			break
		}
	}
	return stop, line < end, true
}

func mergeStyle(base, overlay screen.Style) screen.Style {
	if overlay.Fg.Set() {
		base.Fg = overlay.Fg
	}
	if overlay.Bg.Set() {
		base.Bg = overlay.Bg
	}
	base.Attrs |= overlay.Attrs
	return base
}

func (w *ChatView) selectedMessages() []store.Message {
	if w == nil || !w.focusedMessageSet {
		return nil
	}
	start, end := w.focusStopIndex, w.focusStopIndex
	if w.selectionActive && w.selectionStart >= 0 {
		start = w.selectionStart
	}
	if start > end {
		start, end = end, start
	}
	seen := make(map[string]struct{}, end-start+1)
	messages := make([]store.Message, 0, end-start+1)
	for i := start; i <= end && i < len(w.focusStops); i++ {
		message := w.msgAt(w.focusStops[i].msg)
		key := w.focusStops[i].messageKey
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		messages = append(messages, message)
	}
	return messages
}

func (w *ChatView) focusAtVisible(x, y int) {
	if w == nil || y < 0 || y >= len(w.visibleLines) {
		return
	}
	if w.visibleLines[y].author {
		return
	}
	handle := w.visibleLines[y].msg
	msg := w.msgAt(handle)
	if msg.ID == 0 && msg.Nonce == "" {
		return
	}
	line := w.visibleStart + y
	selected := -1
	for i := range w.focusStops {
		stop := w.focusStops[i]
		if stop.line != line {
			continue
		}
		if stop.kind == chatFocusControl && x >= stop.start && x < stop.end {
			selected = i
			break
		}
		if selected < 0 {
			selected = i
		}
	}
	if selected < 0 {
		for i := range w.focusStops {
			if w.focusStops[i].msg == handle {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return
	}
	previous := w.focusKey
	previousMessage := w.focusedMessage
	stop := w.focusStops[selected]
	nextMessage := w.msgAt(stop.msg)
	w.focusStopIndex = selected
	w.focusStopKey = stop.key
	w.focusedMessage = nextMessage
	w.focusedMessageSet = true
	w.focusedExplicit = true
	if w.onMessageFocus != nil && nextMessage.ID != 0 {
		w.onMessageFocus(nextMessage)
	}
	w.focusKey = stop.messageKey
	if previous != w.focusKey {
		w.activePickerSet = false
		w.activePicker = componentAction{}
		w.invalidateMsgs(previousMessage, nextMessage)
	}
}

// HandleVimFocus lets h/l move between adjacent components before falling back
// to the global focus ring. A collapsed header gets first refusal and unfolds
// in place, keeping the user inside the message instead of leaving the panel.
func (w *ChatView) HandleVimFocus(forward bool) bool {
	if w == nil || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	if w.vimNavigation && w.moveComponent(direction(forward)) {
		return true
	}
	stop := w.focusStops[w.focusStopIndex]
	if stop.kind != chatFocusHeader || stop.headerKey == "" || !w.collapsedHeaders[stop.headerKey] {
		return false
	}
	w.anchorHeaderToggle(stop.headerKey, stop.line-w.visibleStart)
	w.collapsedHeaders[stop.headerKey] = false
	w.invalidateBodies()
	return true
}

func direction(forward bool) int {
	if forward {
		return 1
	}
	return -1
}

// activateAt dispatches the component under (x, y). Shift-clicking an option of
// a single-select flips that control into multi mode: options toggle like
// checkboxes and the picker submits all checked values on Enter or refold.
// Discord strips min/max_values from incoming selects (arikawa never
// unmarshals them), so shift-click is the user's explicit multi override.
func (w *ChatView) activateAt(x, y int, shiftMulti bool) bool {
	if y < 0 || y >= len(w.visibleLines) {
		return false
	}
	// A click inside a video block starts (or, if it is already the playing one,
	// stops) playback.
	for _, h := range w.videoHits {
		if x >= h.x && x < h.x+h.cols && y >= h.y && y < h.y+h.rows {
			if h.url == w.playingVideo {
				w.stopVideoRequest()
				return true
			}
			return w.playVideoHit(h)
		}
	}
	// A click on a loaded image/GIF block opens it enlarged in the viewer.
	if line := w.visibleLines[y]; line.media != nil && !line.media.video() && line.media.img != nil {
		if x >= line.mediaX && x < line.mediaX+line.media.cols {
			if line.media.linkURL != "" {
				w.entityAction = markup.Action{Kind: markup.ActionOpenURL, Target: line.media.linkURL}
				w.entityActionSet = true
				return true
			}
			if w.onOpenMedia != nil {
				w.onOpenMedia(line.media.url, line.media.img, w.mediaFrames(line.media.url))
				return true
			}
		}
	}
	for _, hit := range w.visibleLines[y].entities {
		if x >= hit.start && x < hit.end {
			w.entityAction = hit.action
			w.entityActionSet = true
			return true
		}
	}
	for _, hit := range w.visibleLines[y].actions {
		if x >= hit.start && x < hit.end {
			action := hit.action
			if shiftMulti && action.option && !action.multi && action.kind == store.ComponentSelect {
				w.enableComponentMulti(action)
				action.multi = true
			}
			return w.setComponentAction(action)
		}
	}
	return false
}

func (w *ChatView) scrollUp() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(w.bottomScroll.Offset() + 1)
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

func (w *ChatView) moveFocus(delta int) {
	if !w.focusedExplicit {
		w.ensureInitialFocusedMessage()
		w.focusLatestStop()
	}
	if len(w.focusStops) == 0 {
		if delta < 0 {
			w.scrollUp()
		} else {
			w.scrollDown()
		}
		return
	}
	index := w.focusStopIndex
	if index < 0 || index >= len(w.focusStops) {
		index = 0
	}
	next := index + delta
	if next < 0 || next >= len(w.focusStops) {
		if delta < 0 {
			w.scrollUp()
		} else {
			w.scrollDown()
		}
		return
	}
	w.setFocusStop(next)
	stop := w.focusStops[next]
	start := max(w.renderLineCount-w.viewportHeight-w.bottomScroll.Offset(), 0)
	end := min(start+w.viewportHeight, w.renderLineCount)
	if stop.line < start || stop.line >= end {
		w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight-stop.line-1, 0))
	}
}

func (w *ChatView) focusLatestStop() bool {
	if w == nil || len(w.focusStops) == 0 {
		return false
	}
	key := w.focusKey
	if key == "" {
		return false
	}
	index := -1
	for i := len(w.focusStops) - 1; i >= 0; i-- {
		if w.focusStops[i].messageKey == key {
			index = i
			continue
		}
		if index >= 0 {
			break
		}
	}
	if index < 0 {
		return false
	}
	w.setFocusStop(index)
	return true
}

func (w *ChatView) setFocusStop(index int) {
	if w == nil || index < 0 || index >= len(w.focusStops) {
		return
	}
	previous := w.focusedMessage
	previousMessage := w.focusKey
	stop := w.focusStops[index]
	nextMessage := w.msgAt(stop.msg)
	w.focusStopIndex = index
	w.focusStopKey = stop.key
	w.focusedMessage = nextMessage
	w.focusKey = stop.messageKey
	w.focusedMessageSet = true
	w.focusedExplicit = true
	if w.onMessageFocus != nil && nextMessage.ID != 0 {
		w.onMessageFocus(nextMessage)
	}
	if previousMessage != w.focusKey {
		w.activePickerSet = false
		w.activePicker = componentAction{}
		w.invalidateMsgs(previous, nextMessage)
	}
}

func (w *ChatView) moveComponent(delta int) bool {
	if w == nil || delta == 0 || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	current := w.focusStops[w.focusStopIndex]
	for index := w.focusStopIndex + delta; index >= 0 && index < len(w.focusStops); index += delta {
		candidate := w.focusStops[index]
		if candidate.line != current.line || candidate.messageKey != current.messageKey {
			return false
		}
		if candidate.kind == chatFocusControl {
			w.setFocusStop(index)
			return true
		}
	}
	return false
}

func (w *ChatView) pageUp() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(w.bottomScroll.Offset() + max(w.viewportHeight, 1))
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

func (w *ChatView) pageDown() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(max(w.bottomScroll.Offset()-max(w.viewportHeight, 1), 0))
}

func (w *ChatView) scrollDown() {
	w.vimStickOldest = false
	if w.bottomScroll.Offset() > 0 {
		w.bottomScroll.SetOffset(w.bottomScroll.Offset() - 1)
	}
}

// scrollToOldest implements Vim's gg motion for the loaded transcript. Asking
// for older history at the boundary lets a subsequent gg continue toward the
// channel's true beginning when pagination has more messages available.
func (w *ChatView) scrollToOldest() {
	w.vimStickOldest = true
	w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight, 0))
	if len(w.focusStops) > 0 {
		w.setFocusStop(0)
	}
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

// scrollToNewest implements Vim's G motion.
func (w *ChatView) scrollToNewest() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(0)
	if len(w.focusStops) > 0 {
		w.setFocusStop(len(w.focusStops) - 1)
	}
}

// applyVimBoundaryFocus keeps gg anchored to the true oldest loaded message
// when an asynchronous history page is prepended after the key sequence. It
// also moves the keyboard focus with the viewport, matching Vim's gg motion.
func (w *ChatView) applyVimBoundaryFocus() {
	if w == nil || !w.vimStickOldest {
		return
	}
	w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight, 0))
	if len(w.focusStops) > 0 && w.focusStopIndex != 0 {
		w.setFocusStop(0)
	}
}
