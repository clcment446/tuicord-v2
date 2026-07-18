package tui

import "reflect"

const focusHistoryLimit = 128

// FocusManager owns the ordered keyboard focus ring.
//
// It is a pure state holder: callers decide when to rebuild the ring and how
// to present focus changes to widgets.
type FocusManager struct {
	ring         []Widget
	current      int
	history      []Widget
	historyIndex int
}

// Len returns the number of focusable widgets in the ring.
func (f *FocusManager) Len() int {
	if f == nil {
		return 0
	}
	return len(f.ring)
}

// Register appends a focusable widget to the ring if it is not already present.
func (f *FocusManager) Register(w Widget) {
	if f == nil || !canFocus(w) || f.index(w) >= 0 {
		return
	}
	f.ring = append(f.ring, w)
	if len(f.ring) == 1 {
		f.current = 0
		f.recordVisit(w)
	}
}

// Replace rebuilds the focus ring while preserving the focused widget when it
// is still present and focusable.
func (f *FocusManager) Replace(widgets []Widget) {
	if f == nil {
		return
	}
	prev := f.Focused()
	oldRingLen := len(f.ring)
	f.ring = f.ring[:0]
	f.current = -1
	for _, w := range widgets {
		if canFocus(w) && f.index(w) < 0 {
			f.ring = append(f.ring, w)
		}
	}
	if len(f.ring) < oldRingLen {
		clear(f.ring[len(f.ring):oldRingLen])
	}
	f.pruneHistory()
	if len(f.ring) == 0 {
		return
	}
	f.current = 0
	if prev != nil {
		if f.Set(prev) {
			return
		}
	}
	for i, w := range f.ring {
		preferred, ok := w.(PreferredFocus)
		if ok && preferred.PreferredFocus() {
			f.current = i
			f.recordVisit(w)
			return
		}
	}
	f.recordVisit(f.ring[f.current])
}

// Clear removes every widget from the focus ring.
func (f *FocusManager) Clear() {
	if f == nil {
		return
	}
	clear(f.ring)
	clear(f.history)
	f.ring = nil
	f.history = nil
	f.current = -1
	f.historyIndex = -1
}

// Focused returns the current focus owner, or nil when no widget is focused.
func (f *FocusManager) Focused() Widget {
	if f == nil || len(f.ring) == 0 || f.current < 0 || f.current >= len(f.ring) {
		return nil
	}
	return f.ring[f.current]
}

// Set focuses w when it is present in the focus ring.
func (f *FocusManager) Set(w Widget) bool {
	if f == nil {
		return false
	}
	i := f.index(w)
	if i < 0 {
		return false
	}
	f.current = i
	f.recordVisit(w)
	return true
}

// Next advances focus forward, wrapping at the end of the ring.
func (f *FocusManager) Next() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	if f.current < 0 {
		f.current = 0
		f.recordVisit(f.ring[f.current])
		return f.ring[f.current]
	}
	f.current = (f.current + 1) % len(f.ring)
	f.recordVisit(f.ring[f.current])
	return f.ring[f.current]
}

// Prev moves focus backward, wrapping at the start of the ring.
func (f *FocusManager) Prev() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	if f.current < 0 {
		f.current = 0
		f.recordVisit(f.ring[f.current])
		return f.ring[f.current]
	}
	f.current--
	if f.current < 0 {
		f.current = len(f.ring) - 1
	}
	f.recordVisit(f.ring[f.current])
	return f.ring[f.current]
}

// Back focuses the most recently visited widget before the current one.
// Widgets no longer present in the current focus ring are skipped.
func (f *FocusManager) Back() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	for i := f.historyIndex - 1; i >= 0; i-- {
		if index := f.index(f.history[i]); index >= 0 {
			f.current = index
			f.historyIndex = i
			return f.ring[index]
		}
	}
	return nil
}

// Forward focuses the next widget in the visit history, skipping widgets no
// longer present in the current focus ring.
func (f *FocusManager) Forward() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	for i := f.historyIndex + 1; i < len(f.history); i++ {
		if index := f.index(f.history[i]); index >= 0 {
			f.current = index
			f.historyIndex = i
			return f.ring[index]
		}
	}
	return nil
}

// Remove deletes a widget from the focus ring.
func (f *FocusManager) Remove(w Widget) bool {
	if f == nil {
		return false
	}
	i := f.index(w)
	if i < 0 {
		return false
	}
	wasFocused := i == f.current
	copy(f.ring[i:], f.ring[i+1:])
	f.ring[len(f.ring)-1] = nil
	f.ring = f.ring[:len(f.ring)-1]
	if len(f.ring) == 0 {
		f.current = -1
		f.pruneHistory()
		return true
	}
	if f.current >= len(f.ring) {
		f.current = 0
	} else if i < f.current {
		f.current--
	}
	f.pruneHistory()
	if wasFocused {
		f.recordVisit(f.ring[f.current])
	}
	return true
}

func (f *FocusManager) index(w Widget) int {
	for i, got := range f.ring {
		if sameWidget(got, w) {
			return i
		}
	}
	return -1
}

func (f *FocusManager) recordVisit(w Widget) {
	if f == nil || w == nil {
		return
	}
	if f.historyIndex >= 0 && f.historyIndex < len(f.history) && sameWidget(f.history[f.historyIndex], w) {
		return
	}
	if f.historyIndex+1 < len(f.history) {
		clear(f.history[f.historyIndex+1:])
		f.history = f.history[:f.historyIndex+1]
	}
	f.history = append(f.history, w)
	if len(f.history) > focusHistoryLimit {
		drop := len(f.history) - focusHistoryLimit
		copy(f.history, f.history[drop:])
		clear(f.history[len(f.history)-drop:])
		f.history = f.history[:focusHistoryLimit]
	}
	f.historyIndex = len(f.history) - 1
}

// pruneHistory removes visits to widgets absent from the current ring while
// preserving the cursor's position relative to retained visits.
func (f *FocusManager) pruneHistory() {
	if f == nil {
		return
	}
	oldLen := len(f.history)
	next := 0
	newIndex := -1
	for oldIndex, w := range f.history {
		if f.index(w) < 0 {
			continue
		}
		f.history[next] = w
		if oldIndex <= f.historyIndex {
			newIndex = next
		}
		next++
	}
	clear(f.history[next:oldLen])
	f.history = f.history[:next]
	f.historyIndex = newIndex
}

func canFocus(w Widget) bool {
	if w == nil {
		return false
	}
	f, ok := w.(Focusable)
	return ok && f.CanFocus()
}

func sameWidget(a, b Widget) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	if ta != reflect.TypeOf(b) || !ta.Comparable() {
		return false
	}
	return a == b
}
