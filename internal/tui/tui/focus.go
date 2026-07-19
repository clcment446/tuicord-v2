package tui

import "reflect"

const focusHistoryLimit = 128

// FocusManager owns the ordered keyboard focus ring.
//
// It is a pure state holder apart from its synchronous owner-change callback;
// callers decide when to rebuild the ring and how roots react to transitions.
type FocusManager struct {
	ring         []Widget
	current      int
	history      []Widget
	historyIndex int
	onChange     func(FocusChange)
}

// SetOnChange installs the callback used by App to notify its root. Passing nil
// disables notifications.
func (f *FocusManager) SetOnChange(fn func(FocusChange)) {
	if f != nil {
		f.onChange = fn
	}
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
	prev := f.Focused()
	f.ring = append(f.ring, w)
	if len(f.ring) == 1 {
		f.current = 0
		f.recordVisit(w)
		f.notify(prev, w, FocusChangeDirect)
	}
}

// Replace rebuilds the focus ring while preserving the focused widget when it
// is still present, visible, and focusable.
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
		f.notify(prev, nil, FocusChangeReplace)
		return
	}

	next := 0
	if prev != nil {
		if i := f.index(prev); i >= 0 {
			next = i
			f.current = next
			f.recordVisit(f.ring[next])
			f.notify(prev, f.ring[next], FocusChangeReplace)
			return
		}
	}
	for i, w := range f.ring {
		preferred, ok := w.(PreferredFocus)
		if ok && preferred.PreferredFocus() {
			next = i
			break
		}
	}
	f.current = next
	f.recordVisit(f.ring[next])
	f.notify(prev, f.ring[next], FocusChangeReplace)
}

// Clear removes every widget from the focus ring.
func (f *FocusManager) Clear() {
	if f == nil {
		return
	}
	prev := f.Focused()
	clear(f.ring)
	clear(f.history)
	f.ring = nil
	f.history = nil
	f.current = -1
	f.historyIndex = -1
	f.notify(prev, nil, FocusChangeClear)
}

// Focused returns the current focus owner, or nil when no widget is focused.
func (f *FocusManager) Focused() Widget {
	if f == nil || len(f.ring) == 0 || f.current < 0 || f.current >= len(f.ring) {
		return nil
	}
	return f.ring[f.current]
}

// Set focuses w when it is present in the focus ring and still focusable.
func (f *FocusManager) Set(w Widget) bool {
	return f.set(w, FocusChangeDirect)
}

func (f *FocusManager) set(w Widget, reason FocusChangeReason) bool {
	if f == nil || !canFocus(w) {
		return false
	}
	i := f.index(w)
	if i < 0 {
		return false
	}
	prev := f.Focused()
	f.current = i
	f.recordVisit(w)
	f.notify(prev, w, reason)
	return true
}

// Next advances focus forward, wrapping at the end of the ring.
func (f *FocusManager) Next() Widget {
	return f.step(1, FocusChangeTraversal)
}

// Prev moves focus backward, wrapping at the start of the ring.
func (f *FocusManager) Prev() Widget {
	return f.step(-1, FocusChangeTraversal)
}

func (f *FocusManager) step(delta int, reason FocusChangeReason) Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	start := f.current
	if start < 0 || start >= len(f.ring) {
		start = 0
		if delta < 0 {
			start = len(f.ring) - 1
		}
		if canFocus(f.ring[start]) {
			prev := f.Focused()
			f.current = start
			f.recordVisit(f.ring[start])
			f.notify(prev, f.ring[start], reason)
			return f.ring[start]
		}
	}
	for n := 1; n <= len(f.ring); n++ {
		i := (start + delta*n) % len(f.ring)
		if i < 0 {
			i += len(f.ring)
		}
		if !canFocus(f.ring[i]) {
			continue
		}
		prev := f.Focused()
		f.current = i
		f.recordVisit(f.ring[i])
		f.notify(prev, f.ring[i], reason)
		return f.ring[i]
	}
	return nil
}

// Back focuses the most recently visited widget before the current one.
// Widgets no longer present or focusable in the current ring are skipped.
func (f *FocusManager) Back() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	for i := f.historyIndex - 1; i >= 0; i-- {
		if index := f.index(f.history[i]); index >= 0 && canFocus(f.ring[index]) {
			prev := f.Focused()
			f.current = index
			f.historyIndex = i
			f.notify(prev, f.ring[index], FocusChangeHistory)
			return f.ring[index]
		}
	}
	return nil
}

// Forward focuses the next widget in the visit history, skipping widgets no
// longer present or focusable in the current focus ring.
func (f *FocusManager) Forward() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	for i := f.historyIndex + 1; i < len(f.history); i++ {
		if index := f.index(f.history[i]); index >= 0 && canFocus(f.ring[index]) {
			prev := f.Focused()
			f.current = index
			f.historyIndex = i
			f.notify(prev, f.ring[index], FocusChangeHistory)
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
	prev := f.Focused()
	wasFocused := i == f.current
	copy(f.ring[i:], f.ring[i+1:])
	f.ring[len(f.ring)-1] = nil
	f.ring = f.ring[:len(f.ring)-1]
	if len(f.ring) == 0 {
		f.current = -1
		f.pruneHistory()
		f.notify(prev, nil, FocusChangeRemove)
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
		f.notify(prev, f.ring[f.current], FocusChangeRemove)
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

func (f *FocusManager) notify(previous, current Widget, reason FocusChangeReason) {
	if f == nil || f.onChange == nil {
		return
	}
	if sameWidget(previous, current) {
		// Explicit user/direct assignments still matter to modal state even when
		// a stale ring wraps back to the same owner (for example Tab or a click
		// while composer focus is pending). Passive render preservation is not a
		// focus choice and must remain silent.
		switch reason {
		case FocusChangeDirect, FocusChangeTraversal, FocusChangeHistory, FocusChangePointer:
		default:
			return
		}
	}
	f.onChange(FocusChange{Previous: previous, Current: current, Reason: reason})
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
