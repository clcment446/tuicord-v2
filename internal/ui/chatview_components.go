package ui

import (
	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"time"
)

func componentShortcutRune(ev input.KeyEvent) (rune, bool) {
	if ev.Key != input.KeyRune {
		return 0, false
	}
	switch ev.Rune {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return ev.Rune, true
	case '&':
		return '1', true
	case 'é', 'É':
		return '2', true
	case '"':
		return '3', true
	case '\'':
		return '4', true
	case '(':
		return '5', true
	case 'è', 'È':
		return '7', true
	case '_':
		return '8', true
	case 'ç', 'Ç':
		return '9', true
	case 'à', 'À':
		return '0', true
	default:
		return 0, false
	}
}

func (w *ChatView) activateShortcut(shortcut rune) bool {
	if w == nil || !w.keyboardFocused || !w.focusedMessageSet {
		return false
	}
	for _, line := range w.visibleLines {
		for _, hit := range line.actions {
			if hit.action.shortcut == shortcut && sameMsg(hit.action.message, w.focusedMessage) {
				return w.setComponentAction(hit.action)
			}
		}
	}
	return false
}

func (w *ChatView) foldFocusedHeader() bool {
	if w == nil || !w.keyboardFocused || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	stop := w.focusStops[w.focusStopIndex]
	if stop.kind != chatFocusHeader || stop.headerKey == "" {
		return false
	}
	if w.collapsedHeaders == nil {
		w.collapsedHeaders = map[string]bool{}
	}
	w.anchorHeaderToggle(stop.headerKey, stop.line-w.visibleStart)
	w.collapsedHeaders[stop.headerKey] = !w.collapsedHeaders[stop.headerKey]
	w.invalidateBodies()
	return true
}

func (w *ChatView) enableComponentMulti(action componentAction) {
	if w.multiPickers == nil {
		w.multiPickers = map[string]bool{}
	}
	w.multiPickers[action.controlKey()] = true
	w.invalidateBodies()
}

func (w *ChatView) componentMultiEnabled(controlKey string) bool {
	return w.multiPickers[controlKey]
}

func (w *ChatView) setComponentAction(action componentAction) bool {
	if action.disabled {
		return false
	}
	// Every path below changes how the component draws: the flash, the
	// expansion, or the selection.
	w.invalidateBodies()
	key := action.key()
	if w.componentFlashes == nil {
		w.componentFlashes = map[string]time.Time{}
	}
	w.componentFlashes[key] = time.Now().Add(500 * time.Millisecond)
	if action.option {
		w.setComponentSelection(action)
	} else if action.expandable {
		if w.expandedComponents == nil {
			w.expandedComponents = map[string]bool{}
		}
		key := action.controlKey()
		w.anchorControlToggle(key)
		if w.expandedComponents[key] {
			w.expandedComponents[key] = false
			return w.submitComponentPicker(action)
		}
		w.expandedComponents[key] = true
		w.activePicker = action
		w.activePickerSet = true
		return true
	}
	if action.option {
		w.activePicker = action
		w.activePickerSet = true
		if action.multi {
			return true
		}
		if w.expandedComponents != nil {
			w.anchorControlToggle(action.controlKey())
			w.expandedComponents[action.controlKey()] = false
		}
		return w.submitComponentPicker(action)
	}
	w.componentAction = ComponentAction{
		Shortcut: action.shortcut,
		CustomID: action.customID,
		Label:    action.label,
		Kind:     action.kind,
		RawType:  action.rawType,
		Value:    action.value,
		URL:      action.url,
		Message:  action.message,
	}
	w.componentActionSet = true
	return true
}

func (w *ChatView) setComponentSelection(action componentAction) {
	w.invalidateBodies()
	if w.selectedComponents == nil {
		w.selectedComponents = map[string]map[string]bool{}
	}
	key := action.controlKey()
	if !action.multi {
		w.selectedComponents[key] = map[string]bool{action.value: true}
		return
	}
	selected := w.selectedComponents[key]
	if selected == nil {
		selected = componentValuesMap(action.defaults)
		w.selectedComponents[key] = selected
	}
	if selected[action.value] {
		delete(selected, action.value)
	} else {
		selected[action.value] = true
	}
}

func (w *ChatView) submitActiveComponentPicker() bool {
	if !w.activePickerSet {
		return false
	}
	action := w.activePicker
	if w.expandedComponents != nil && !w.expandedComponents[action.controlKey()] {
		return false
	}
	if w.expandedComponents != nil {
		w.anchorControlToggle(action.controlKey())
		w.expandedComponents[action.controlKey()] = false
		w.invalidateBodies()
	}
	return w.submitComponentPicker(action)
}

func (w *ChatView) submitComponentPicker(action componentAction) bool {
	if action.disabled || (!action.expandable && !action.option) {
		return false
	}
	multi := action.multi || w.componentMultiEnabled(action.controlKey())
	values := w.componentSelectedValues(action)
	label := action.label
	if action.option && multi && action.controlLabel != "" {
		label = action.controlLabel
	}
	value := action.value
	if multi || !action.option {
		value = ""
		if len(values) == 1 {
			value = values[0]
		}
	}
	delete(w.multiPickers, action.controlKey())
	w.componentAction = ComponentAction{
		Shortcut: action.shortcut,
		CustomID: action.customID,
		Label:    label,
		Kind:     action.kind,
		RawType:  action.rawType,
		Value:    value,
		Values:   values,
		URL:      action.url,
		Message:  action.message,
	}
	w.componentActionSet = true
	return true
}

func (w *ChatView) componentSelectedValues(action componentAction) []string {
	selected, ok := w.selectedComponents[action.controlKey()]
	if !ok {
		selected = componentValuesMap(action.defaults)
	}
	var values []string
	seen := map[string]bool{}
	for _, opt := range action.options {
		value := componentOptionValue(opt)
		if selected[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	for value := range selected {
		if !seen[value] {
			values = append(values, value)
		}
	}
	if len(values) == 0 && action.option && action.value != "" && !action.multi {
		values = append(values, action.value)
	}
	return values
}

func componentValuesMap(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func (w *ChatView) expireComponentFlashes(now time.Time) bool {
	if len(w.componentFlashes) == 0 {
		return false
	}
	changed := false
	for key, until := range w.componentFlashes {
		if !now.Before(until) {
			delete(w.componentFlashes, key)
			changed = true
		}
	}
	if changed {
		w.invalidateBodies()
	}
	return changed
}

// TakeEntityAction returns a clicked user/role mention action.
func (w *ChatView) TakeEntityAction() (markup.Action, bool) {
	if w == nil || !w.entityActionSet {
		return markup.Action{}, false
	}
	a := w.entityAction
	w.entityActionSet = false
	return a, true
}

// TakeContextMessage returns the message most recently right-clicked during the
// current event bubble. It clears the pending value so one click opens one menu.
func (w *ChatView) TakeContextMessage() (store.Message, bool) {
	if w == nil || !w.contextMessageSet {
		return store.Message{}, false
	}
	msg := w.contextMessage
	w.contextMessage = store.Message{}
	w.contextMessageSet = false
	return msg, true
}

// TakeComponentAction returns the most recent button/select activation captured
// by mouse or numeric shortcut. Live Discord submission is handled above ChatView.
func (w *ChatView) TakeComponentAction() (ComponentAction, bool) {
	if w == nil || !w.componentActionSet {
		return ComponentAction{}, false
	}
	action := w.componentAction
	w.componentAction = ComponentAction{}
	w.componentActionSet = false
	return action, true
}
