package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
)

type componentAction struct {
	message      store.Message
	defaults     []string
	options      []store.ComponentOption
	customID     string
	label        string
	controlLabel string
	value        string
	url          string
	rawType      int
	kind         store.ComponentKind
	shortcut     rune
	disabled     bool
	expandable   bool
	option       bool
	multi        bool
}

func (a componentAction) key() string {
	key := a.controlKey()
	if a.option {
		key += ":option:" + a.value + ":" + a.label
	}
	return key
}

func (a componentAction) controlKey() string {
	target := a.customID
	if target == "" {
		target = a.url
	}
	if target == "" {
		target = a.label
	}
	var buf [96]byte
	out := strconv.AppendUint(buf[:0], uint64(a.message.ChannelID), 10)
	out = append(out, ':')
	if a.message.ID != 0 {
		out = strconv.AppendUint(out, uint64(a.message.ID), 10)
	} else {
		out = append(out, "pending:"...)
		out = append(out, a.message.Nonce...)
	}
	out = append(out, ':')
	out = strconv.AppendInt(out, int64(a.kind), 10)
	out = append(out, ':')
	out = append(out, target...)
	return string(out)
}

type componentHit struct {
	action     componentAction
	key        string
	start, end int
}

// ComponentAction is the public description of the last component the user
// activated in ChatView. It is intentionally store-shaped so examples can use it
// without importing Discord types.
type ComponentAction struct {
	Shortcut rune
	CustomID string
	Label    string
	Kind     store.ComponentKind
	// RawType is Discord's numeric component type, needed to address the
	// interaction (entity selects share Kind but not wire type).
	RawType int
	Value   string
	Values  []string
	URL     string
	Message store.Message
}

func (w *ChatView) renderComponentTree(m store.Message, width int, base screen.Style) []chatLine {
	tree := m.ComponentTree
	if len(tree) == 0 && len(m.Components) > 0 {
		tree = legacyComponentTree(m.Components)
	}
	if len(tree) == 0 {
		return nil
	}
	ctx := componentRenderContext{hideShortcuts: !w.componentShortcutsVisible(m)}
	var lines []chatLine
	for _, node := range tree {
		lines = append(lines, w.renderComponentNode(&ctx, m, node, width, base, componentFrame{})...)
	}
	return lines
}

type componentRenderContext struct {
	shortcutIndex int
	hideShortcuts bool
}

var componentShortcuts = []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9', '0'}

func (ctx *componentRenderContext) nextShortcut() rune {
	if ctx.hideShortcuts {
		return 0
	}
	if ctx.shortcutIndex >= len(componentShortcuts) {
		return 0
	}
	shortcut := componentShortcuts[ctx.shortcutIndex]
	ctx.shortcutIndex++
	return shortcut
}

func (w *ChatView) componentShortcutsVisible(message store.Message) bool {
	return w != nil && w.keyboardFocused && w.focusedMessageSet &&
		messagePlacementPrefix(w.focusedMessage) == messagePlacementPrefix(message)
}

type componentFrame struct {
	prefix string
	style  screen.Style
}

func (w *ChatView) renderComponentNode(ctx *componentRenderContext, m store.Message, node store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	switch node.Kind {
	case store.ComponentContainer:
		return w.renderComponentContainer(ctx, m, node, width, base, frame)
	case store.ComponentSection:
		return w.renderComponentSection(ctx, m, node, width, base, frame)
	case store.ComponentActionRow:
		return w.renderComponentControls(ctx, m, node.Children, width, base, frame)
	case store.ComponentButton, store.ComponentLinkButton, store.ComponentSelect,
		store.ComponentRadioGroup, store.ComponentCheckboxGroup, store.ComponentCheckbox,
		store.ComponentFileUpload, store.ComponentTextInput:
		return w.renderComponentControls(ctx, m, []store.ComponentNode{node}, width, base, frame)
	case store.ComponentLabel:
		return w.renderComponentLabel(ctx, m, node, width, base, frame)
	case store.ComponentTextDisplay:
		return w.renderComponentText(node.Content, width, base, frame)
	case store.ComponentThumbnail:
		return []chatLine{componentTextLine(frame, componentMediaLabel(node, "thumbnail"), mergeStyle(base, w.styles.Cell("components.description")))}
	case store.ComponentMediaGallery:
		return w.renderComponentGallery(node, base, frame)
	case store.ComponentFile:
		return []chatLine{componentTextLine(frame, componentMediaLabel(node, "file"), mergeStyle(base, w.styles.Cell("components.description")))}
	case store.ComponentSeparator:
		return renderComponentSeparator(node, width, base, frame)
	case store.ComponentUnknown:
		return []chatLine{componentTextLine(frame, fmt.Sprintf("[unknown component type %d]", node.RawType), mergeStyle(base, w.styles.Cell("components.disabled")))}
	default:
		return []chatLine{componentTextLine(frame, fmt.Sprintf("[component: %d]", node.RawType), mergeStyle(base, w.styles.Cell("components.disabled")))}
	}
}

func (w *ChatView) renderComponentLabel(ctx *componentRenderContext, m store.Message, node store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	var lines []chatLine
	if node.Label != "" {
		label := node.Label
		if node.Required {
			label += " *"
		}
		lines = append(lines, componentTextLine(frame, label, mergeStyle(base, w.styles.Cell("components.label"))))
	}
	if node.Description != "" {
		lines = append(lines, componentTextLine(frame, node.Description, mergeStyle(base, w.styles.Cell("components.description"))))
	}
	for _, child := range node.Children {
		lines = append(lines, w.renderComponentNode(ctx, m, child, width, base, frame)...)
	}
	return lines
}

func (w *ChatView) renderComponentContainer(ctx *componentRenderContext, m store.Message, node store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	accent := w.componentAccent(node, frame)
	bg := w.embedBackground(accent, base)
	borderStyle := base
	borderStyle.Fg = accent
	borderStyle.Bg = bg
	contentBase := base
	contentBase.Bg = bg
	inner := max(width-2, 1)

	var lines []chatLine
	if node.Spoiler {
		lines = append(lines, componentTextLine(componentFrame{}, "[spoiler container]", mergeStyle(contentBase, w.styles.Cell("components.description"))))
	}
	for _, child := range node.Children {
		lines = append(lines, w.renderComponentNode(ctx, m, child, inner, contentBase, componentFrame{})...)
	}
	if len(lines) == 0 {
		lines = append(lines, componentTextLine(componentFrame{}, "[container]", mergeStyle(contentBase, w.styles.Cell("components.description"))))
	}
	return frameEmbedLines(lines, inner, borderStyle, contentBase)
}

func (w *ChatView) componentAccent(node store.ComponentNode, frame componentFrame) screen.Color {
	if !w.styles.HasCustom("components.border") && node.AccentColor != 0 {
		return rgbColor(node.AccentColor)
	}
	if frame.style.Fg.Set() {
		return frame.style.Fg
	}
	if configured := w.styles.Cell("components.border"); configured.Fg.Set() {
		return configured.Fg
	}
	return screen.RGB(88, 101, 242)
}

func (w *ChatView) renderComponentSection(ctx *componentRenderContext, m store.Message, node store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	var lines []chatLine
	accessory := ""
	if node.Accessory != nil && node.Accessory.Kind != store.ComponentButton &&
		node.Accessory.Kind != store.ComponentLinkButton && node.Accessory.Kind != store.ComponentSelect {
		accessory = componentAccessoryLabel(*node.Accessory)
	}
	for i, child := range node.Children {
		childLines := w.renderComponentNode(ctx, m, child, width, base, frame)
		if i == 0 && accessory != "" && len(childLines) > 0 && width >= 42 {
			childLines[0].segments = append(childLines[0].segments, chatSegment{text: "  " + accessory, style: mergeStyle(base, w.styles.Cell("components.description"))})
		}
		lines = append(lines, childLines...)
	}
	if node.Accessory != nil && (node.Accessory.Kind == store.ComponentButton || node.Accessory.Kind == store.ComponentLinkButton || node.Accessory.Kind == store.ComponentSelect) {
		lines = append(lines, w.renderComponentNode(ctx, m, *node.Accessory, width, base, frame)...)
		return lines
	}
	if accessory != "" && (len(node.Children) == 0 || width < 42) {
		if node.Accessory.Kind == store.ComponentButton || node.Accessory.Kind == store.ComponentLinkButton || node.Accessory.Kind == store.ComponentSelect {
			lines = append(lines, w.renderComponentNode(ctx, m, *node.Accessory, width, base, frame)...)
		} else {
			lines = append(lines, componentTextLine(frame, accessory, mergeStyle(base, w.styles.Cell("components.description"))))
		}
	}
	return lines
}

func (w *ChatView) renderComponentText(content string, width int, base screen.Style, frame componentFrame) []chatLine {
	if content == "" {
		return nil
	}
	inner := max(width-text.Width(frame.prefix), 1)
	lines := w.renderContent(content, inner, base)
	for i := range lines {
		lines[i] = prependComponentFrame(lines[i], frame)
	}
	return lines
}

func (w *ChatView) renderComponentGallery(node store.ComponentNode, base screen.Style, frame componentFrame) []chatLine {
	if len(node.Media) == 0 {
		return []chatLine{componentTextLine(frame, "[media gallery]", mergeStyle(base, w.styles.Cell("components.description")))}
	}
	var lines []chatLine
	for i, media := range node.Media {
		label := media.Description
		if label == "" {
			label = media.Name
		}
		if label == "" {
			label = media.URL
		}
		if label == "" {
			label = "media"
		}
		if media.Spoiler {
			label = "spoiler: " + label
		}
		lines = append(lines, componentTextLine(frame, fmt.Sprintf("▒▒ media %d/%d: %s ▒▒", i+1, len(node.Media), label), mergeStyle(base, w.styles.Cell("components.description"))))
	}
	return lines
}

func renderComponentSeparator(node store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	count := 1
	if node.Spacing == 2 {
		count = 2
	}
	var lines []chatLine
	for i := 0; i < count; i++ {
		if node.Divider || node.Spacing == 0 {
			available := max(width-text.Width(frame.prefix), 1)
			lines = append(lines, componentTextLine(frame, strings.Repeat("─", min(available, 32)), base))
		} else {
			lines = append(lines, componentTextLine(frame, "", base))
		}
	}
	return lines
}

func (w *ChatView) renderComponentControls(ctx *componentRenderContext, m store.Message, nodes []store.ComponentNode, width int, base screen.Style, frame componentFrame) []chatLine {
	var lines []chatLine
	var expanded []store.ComponentNode
	prefixWidth := text.Width(frame.prefix)
	line := componentTextLine(frame, "", base)
	x := prefixWidth
	flush := func() {
		lines = append(lines, line)
		line = componentTextLine(frame, "", base)
		x = prefixWidth
	}
	for i, node := range nodes {
		gap := 0
		if i > 0 && x > prefixWidth {
			gap = 1
		}
		action := componentAction{
			customID:   node.CustomID,
			label:      componentControlLabel(node),
			kind:       node.Kind,
			rawType:    node.RawType,
			disabled:   node.Disabled,
			expandable: componentIsFoldable(node),
			multi:      componentIsMultiSelect(node),
			defaults:   componentInitialSelectedValues(node),
			options:    node.Options,
			url:        node.URL,
			message:    m,
		}
		if !node.Disabled {
			action.shortcut = ctx.nextShortcut()
		}
		expandedNow := w.componentExpanded(action)
		chip := componentControlChip(node, action.shortcut, expandedNow)
		if width > 0 {
			chip = text.Truncate(chip, max(width-prefixWidth, 0), text.Ellipsis)
		}
		chipWidth := text.Width(chip)
		if width > 0 && x > prefixWidth && x+gap+chipWidth > width {
			flush()
			gap = 0
		}
		if gap > 0 {
			line.segments = append(line.segments, chatSegment{text: " ", style: base})
			x++
		}
		style := w.componentControlStyle(node, action, base)
		line.segments = append(line.segments, chatSegment{text: chip, style: style})
		if !action.disabled && chipWidth > 0 {
			line.actions = append(line.actions, componentHit{key: action.key(), action: action, start: x, end: x + chipWidth})
		}
		x += chipWidth
		if expandedNow {
			expanded = append(expanded, node)
		}
	}
	if len(line.segments) > 0 || len(line.actions) > 0 {
		flush()
	}
	for _, node := range expanded {
		action := componentAction{
			customID:   node.CustomID,
			label:      componentControlLabel(node),
			kind:       node.Kind,
			rawType:    node.RawType,
			disabled:   node.Disabled,
			expandable: true,
			multi:      componentIsMultiSelect(node),
			defaults:   componentInitialSelectedValues(node),
			options:    node.Options,
			message:    m,
		}
		lines = append(lines, w.renderComponentOptions(ctx, m, node, action, width, base, frame)...)
	}
	return lines
}

func (w *ChatView) componentControlStyle(node store.ComponentNode, action componentAction, base screen.Style) screen.Style {
	style := base
	switch node.Style {
	case 1:
		style = mergeStyle(style, w.styles.Cell("components.button"))
	case 3:
		style = mergeStyle(style, w.styles.Cell("components.success"))
	case 4:
		style = mergeStyle(style, w.styles.Cell("components.error"))
	}
	if node.Kind == store.ComponentLinkButton {
		style = mergeStyle(style, w.styles.Cell("components.link"))
	}
	if node.Disabled {
		style = mergeStyle(style, w.styles.Cell("components.button.disabled"))
	}
	if w.componentFlashing(action) {
		style = mergeStyle(style, w.styles.Cell("components.disabled"))
	}
	switch node.State {
	case store.ComponentStatePending:
		style = mergeStyle(style, w.styles.Cell("components.pending"))
	case store.ComponentStateSuccess:
		style = mergeStyle(style, w.styles.Cell("components.success"))
	case store.ComponentStateError:
		style = mergeStyle(style, w.styles.Cell("components.error"))
	}
	return style
}

func (w *ChatView) renderComponentOptions(ctx *componentRenderContext, m store.Message, node store.ComponentNode, parent componentAction, width int, base screen.Style, frame componentFrame) []chatLine {
	options := componentOptions(node)
	if len(options) == 0 {
		return []chatLine{componentTextLine(frame, "  (no choices)", mergeStyle(base, w.styles.Cell("components.description")))}
	}
	if w.componentMultiEnabled(parent.controlKey()) {
		parent.multi = true
	}
	var lines []chatLine
	for _, opt := range options {
		value := componentOptionValue(opt)
		action := componentAction{
			shortcut:     ctx.nextShortcut(),
			customID:     parent.customID,
			label:        opt.Label,
			controlLabel: parent.label,
			kind:         parent.kind,
			rawType:      parent.rawType,
			option:       true,
			multi:        parent.multi,
			value:        value,
			defaults:     parent.defaults,
			options:      parent.options,
			message:      m,
		}
		marker := w.componentOptionMarker(node, parent, opt)
		label := opt.Label
		if opt.Description != "" {
			label += " — " + opt.Description
		}
		if action.shortcut != 0 {
			label = fmt.Sprintf("%c %s", action.shortcut, label)
		}
		content := "  " + marker + " " + label
		if width > 0 {
			content = text.Truncate(content, max(width-text.Width(frame.prefix), 0), text.Ellipsis)
		}
		style := base
		if w.componentFlashing(action) {
			style = mergeStyle(style, w.styles.Cell("components.disabled"))
		}
		line := componentTextLine(frame, content, style)
		start := text.Width(frame.prefix)
		if text.Width(content) > 0 {
			line.actions = append(line.actions, componentHit{
				key:    action.key(),
				action: action,
				start:  start,
				end:    start + text.Width(content),
			})
		}
		lines = append(lines, line)
	}
	return lines
}

func (w *ChatView) componentOptionMarker(node store.ComponentNode, parent componentAction, opt store.ComponentOption) string {
	selected := w.componentOptionSelected(node, parent, opt)
	switch node.Kind {
	case store.ComponentCheckboxGroup:
		if selected {
			return "☑"
		}
		return "☐"
	case store.ComponentSelect:
		if parent.multi {
			if selected {
				return "☑"
			}
			return "☐"
		}
		if selected {
			return "●"
		}
		return "•"
	case store.ComponentRadioGroup:
		if selected {
			return "●"
		}
		return "○"
	default:
		if selected {
			return "●"
		}
		return "•"
	}
}

func (w *ChatView) componentOptionSelected(node store.ComponentNode, parent componentAction, opt store.ComponentOption) bool {
	value := componentOptionValue(opt)
	if selected, ok := w.selectedComponents[parent.controlKey()]; ok {
		return selected[value]
	}
	if node.Value != "" && node.Value == value {
		return true
	}
	if slices.Contains(node.Values, value) {
		return true
	}
	return opt.Default
}

func componentControlChip(node store.ComponentNode, shortcut rune, expanded bool) string {
	label := componentControlLabel(node)
	if shortcut != 0 {
		label = fmt.Sprintf("%c %s", shortcut, label)
	}
	switch node.State {
	case store.ComponentStatePending:
		label += " ..."
	case store.ComponentStateSuccess:
		label += " ok"
	case store.ComponentStateError:
		label += " failed"
	}
	if node.Disabled {
		label += " disabled"
	}
	if componentIsList(node) {
		icon := "▸"
		if expanded {
			icon = "▾"
		}
		return icon + " " + label
	}
	if node.Kind == store.ComponentCheckbox {
		if node.Value == "true" {
			return "☑ " + label
		}
		return "☐ " + label
	}
	if node.InputField {
		return "[" + label + "]"
	}
	return "⟦ " + label + " ⟧"
}

func componentControlLabel(node store.ComponentNode) string {
	if node.Label != "" {
		return node.Label
	}
	if node.Placeholder != "" {
		return node.Placeholder
	}
	if node.Kind == store.ComponentSelect {
		return "Select"
	}
	if node.Kind == store.ComponentRadioGroup {
		return "Choose one"
	}
	if node.Kind == store.ComponentCheckboxGroup {
		return "Choose options"
	}
	if node.Kind == store.ComponentFileUpload {
		return "Upload file"
	}
	if node.Kind == store.ComponentTextInput {
		if node.Placeholder != "" {
			return node.Placeholder
		}
		return "Input"
	}
	if node.Kind == store.ComponentCheckbox {
		return "Checkbox"
	}
	if node.Kind == store.ComponentLinkButton {
		return "Open"
	}
	return "Button"
}

func componentIsFoldable(node store.ComponentNode) bool {
	return componentIsList(node) && len(componentOptions(node)) > 0
}

func componentIsList(node store.ComponentNode) bool {
	switch node.Kind {
	case store.ComponentSelect, store.ComponentRadioGroup, store.ComponentCheckboxGroup:
		return true
	default:
		return false
	}
}

func componentOptions(node store.ComponentNode) []store.ComponentOption {
	return node.Options
}

func componentInitialSelectedValues(node store.ComponentNode) []string {
	seen := map[string]bool{}
	var values []string
	add := func(value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		values = append(values, value)
	}
	add(node.Value)
	for _, value := range node.Values {
		add(value)
	}
	for _, opt := range node.Options {
		if opt.Default {
			add(componentOptionValue(opt))
		}
	}
	return values
}

func (w *ChatView) componentExpanded(action componentAction) bool {
	if !action.expandable || w.expandedComponents == nil {
		return false
	}
	return w.expandedComponents[action.controlKey()]
}

func (w *ChatView) componentFlashing(action componentAction) bool {
	if w.componentFlashes == nil {
		return false
	}
	until, ok := w.componentFlashes[action.key()]
	return ok && time.Now().Before(until)
}

func componentIsMultiSelect(node store.ComponentNode) bool {
	return node.Kind == store.ComponentCheckboxGroup || node.MaxValues > 1
}

func componentOptionValue(opt store.ComponentOption) string {
	if opt.Value != "" {
		return opt.Value
	}
	return opt.Label
}

func componentAccessoryLabel(node store.ComponentNode) string {
	switch node.Kind {
	case store.ComponentThumbnail:
		return componentMediaLabel(node, "thumbnail")
	case store.ComponentButton, store.ComponentLinkButton:
		return componentControlChip(node, 0, false)
	default:
		return fmt.Sprintf("[component: %d]", node.RawType)
	}
}

func componentMediaLabel(node store.ComponentNode, fallback string) string {
	if len(node.Media) == 0 {
		return "[" + fallback + "]"
	}
	media := node.Media[0]
	label := media.Description
	if label == "" {
		label = media.Name
	}
	if label == "" {
		label = media.URL
	}
	if label == "" {
		label = fallback
	}
	if media.Spoiler {
		label = "spoiler: " + label
	}
	return "▒▒ " + label + " ▒▒"
}

func componentTextLine(frame componentFrame, s string, style screen.Style) chatLine {
	line := chatLine{}
	if frame.prefix != "" {
		line.segments = append(line.segments, chatSegment{text: frame.prefix, style: frame.style})
	}
	if s != "" {
		line.segments = append(line.segments, chatSegment{text: s, style: style})
	}
	return line
}

func prependComponentFrame(line chatLine, frame componentFrame) chatLine {
	if frame.prefix == "" {
		return line
	}
	next := translateChatLine(line, text.Width(frame.prefix))
	next.segments = next.segments[:0]
	next.segments = append(next.segments, chatSegment{text: frame.prefix, style: frame.style})
	next.segments = append(next.segments, line.segments...)
	next.text = frame.prefix + line.text
	return next
}

func legacyComponentTree(components []store.Component) []store.ComponentNode {
	row := store.ComponentNode{Kind: store.ComponentActionRow, RawType: 1}
	for _, comp := range components {
		node := store.ComponentNode{
			Kind:     comp.Kind,
			RawType:  int(comp.Kind),
			CustomID: comp.CustomID,
			Label:    comp.Label,
			Style:    comp.Style,
			URL:      comp.URL,
			Disabled: comp.Disabled,
		}
		row.Children = append(row.Children, node)
	}
	if len(row.Children) == 0 {
		return nil
	}
	return []store.ComponentNode{row}
}
