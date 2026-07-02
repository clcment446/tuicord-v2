package tui

import "reflect"

// FocusManager owns the ordered keyboard focus ring.
//
// It is a pure state holder: callers decide when to rebuild the ring and how
// to present focus changes to widgets.
type FocusManager struct {
	ring    []Widget
	current int
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
	}
}

// Replace rebuilds the focus ring while preserving the focused widget when it
// is still present and focusable.
func (f *FocusManager) Replace(widgets []Widget) {
	if f == nil {
		return
	}
	prev := f.Focused()
	f.ring = f.ring[:0]
	f.current = -1
	for _, w := range widgets {
		if canFocus(w) && f.index(w) < 0 {
			f.ring = append(f.ring, w)
		}
	}
	if len(f.ring) == 0 {
		return
	}
	f.current = 0
	if prev != nil {
		_ = f.Set(prev)
	}
}

// Clear removes every widget from the focus ring.
func (f *FocusManager) Clear() {
	if f == nil {
		return
	}
	f.ring = nil
	f.current = -1
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
	return true
}

// Next advances focus forward, wrapping at the end of the ring.
func (f *FocusManager) Next() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	if f.current < 0 {
		f.current = 0
		return f.ring[f.current]
	}
	f.current = (f.current + 1) % len(f.ring)
	return f.ring[f.current]
}

// Prev moves focus backward, wrapping at the start of the ring.
func (f *FocusManager) Prev() Widget {
	if f == nil || len(f.ring) == 0 {
		return nil
	}
	if f.current < 0 {
		f.current = 0
		return f.ring[f.current]
	}
	f.current--
	if f.current < 0 {
		f.current = len(f.ring) - 1
	}
	return f.ring[f.current]
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
	f.ring = append(f.ring[:i], f.ring[i+1:]...)
	if len(f.ring) == 0 {
		f.current = -1
		return true
	}
	if f.current >= len(f.ring) {
		f.current = 0
	} else if i < f.current {
		f.current--
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
