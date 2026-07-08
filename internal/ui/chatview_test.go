package ui

import (
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func rowText(buf *screen.Buffer, y int) string {
	var b strings.Builder
	for x := 0; x < buf.Width(); x++ {
		b.WriteString(buf.Cell(x, y).Content)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestChatViewRendersBottomAligned(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Content: "first"})
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "second"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	// 2 messages × (author + 1 content line) = 4 lines, fills the 4-row region.
	got := []string{rowText(buf, 0), rowText(buf, 1), rowText(buf, 2), rowText(buf, 3)}
	want := []string{"alice", "first", "bob", "second"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChatViewMarksPendingAndFailed(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "you", Content: "hi", Pending: true})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if !strings.Contains(rowText(buf, 0), "sending") {
		t.Errorf("pending header = %q, want to contain 'sending'", rowText(buf, 0))
	}
}

func TestChatViewResolvesMarkup(t *testing.T) {
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "hi <@42> **bold**"})

	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	// Mention resolved to @alice, bold delimiters stripped.
	if got := rowText(buf, 1); got != "hi @alice bold" {
		t.Errorf("content row = %q, want %q", got, "hi @alice bold")
	}
	if got := buf.Cell(3, 1).Style.Fg; got != (screen.Color{}) {
		t.Errorf("mention style fg = %+v, want default without configured accent", got)
	}
	if got := buf.Cell(10, 1).Style.Attrs & screen.Bold; got == 0 {
		t.Fatal("bold span was not drawn bold")
	}
}

func TestChatViewRendersStackedMarkup(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "**__text__** ~~*gone*~~"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 1); got != "text gone" {
		t.Fatalf("content row = %q, want %q", got, "text gone")
	}
	first := buf.Cell(0, 1).Style.Attrs
	if first&screen.Bold == 0 || first&screen.Underline == 0 {
		t.Fatalf("stacked bold/underline attrs = %v", first)
	}
	second := buf.Cell(5, 1).Style.Attrs
	if second&screen.Strike == 0 || second&screen.Italic == 0 {
		t.Fatalf("stacked strike/italic attrs = %v", second)
	}
}

func TestChatViewRendersComponentsV2Tree(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        21,
		ChannelID: 1,
		Author:    "dao",
		ComponentTree: []store.ComponentNode{{
			Kind:        store.ComponentContainer,
			AccentColor: 0x5865F2,
			Children: []store.ComponentNode{
				{Kind: store.ComponentTextDisplay, Content: "**Heavenly Dao** status"},
				{
					Kind: store.ComponentSection,
					Children: []store.ComponentNode{
						{Kind: store.ComponentTextDisplay, Content: "Qi: 120 / 200"},
					},
					Accessory: &store.ComponentNode{
						Kind:     store.ComponentButton,
						Label:    "Cultivate",
						CustomID: "cultivate",
						Style:    1,
					},
				},
				{Kind: store.ComponentMediaGallery, Media: []store.ComponentMedia{{Description: "realm art"}}},
				{Kind: store.ComponentSeparator, Divider: true, Spacing: 1},
				{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
					{Kind: store.ComponentButton, Label: "Breakthrough", CustomID: "break", Style: 3},
					{Kind: store.ComponentSelect, Placeholder: "Choose technique", CustomID: "technique", Options: []store.ComponentOption{
						{Label: "Heaven", Value: "heaven", Description: "SSS"},
						{Label: "Hope", Value: "hope", Description: "SS"},
						{Label: "Light", Value: "light", Description: "S"},
					}},
				}},
			},
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(80, 12)
	view.Draw(buf.Clip(buf.Bounds()))
	all := rowsText(buf)

	for _, want := range []string{"╭", "│Heavenly Dao status", "│Qi: 120 / 200", "realm art", "1 Cultivate", "2 Breakthrough", "▸ 3 Choose technique", "╰"} {
		if !strings.Contains(all, want) {
			t.Fatalf("rendered rows missing %q:\n%s", want, all)
		}
	}
	if border := buf.Cell(0, 1).Style; !border.Bg.Set() || border.Bg == rgbColor(0x5865F2) {
		t.Fatalf("component border style = %+v, want subtle tinted bg", border)
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '2'}) {
		t.Fatal("component shortcut was not handled")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.CustomID != "break" || action.Label != "Breakthrough" {
		t.Fatalf("component action = %+v,%v", action, ok)
	}
	if len(view.componentFlashes) != 1 {
		t.Fatalf("component flashes = %d, want 1", len(view.componentFlashes))
	}
	if !view.expireComponentFlashes(time.Now().Add(time.Second)) || len(view.componentFlashes) != 0 {
		t.Fatalf("component flash did not expire: %d", len(view.componentFlashes))
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '3'}) {
		t.Fatal("select shortcut was not handled")
	}
	if action, ok = view.TakeComponentAction(); ok {
		t.Fatalf("opening a select should not send an action, got %+v", action)
	}
	view.Draw(buf.Clip(buf.Bounds()))
	all = rowsText(buf)
	for _, want := range []string{"▾ 3 Choose technique", "• 4 Heaven — SSS", "• 5 Hope — SS", "• 6 Light — S"} {
		if !strings.Contains(all, want) {
			t.Fatalf("expanded rows missing %q:\n%s", want, all)
		}
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '4'}) {
		t.Fatal("select option shortcut was not handled")
	}
	action, ok = view.TakeComponentAction()
	if !ok || action.CustomID != "technique" || action.Label != "Heaven" || action.Value != "heaven" {
		t.Fatalf("select option action = %+v,%v", action, ok)
	}
}

func TestChatViewMultiSelectSubmitsSelectedValuesOnEnterOrRefold(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        24,
		ChannelID: 1,
		Author:    "shop",
		ComponentTree: []store.ComponentNode{{
			Kind:        store.ComponentContainer,
			AccentColor: 0x57F287,
			Children: []store.ComponentNode{{
				Kind:        store.ComponentSelect,
				Placeholder: "Choose one or more items to sell",
				CustomID:    "sell_items",
				MinValues:   1,
				MaxValues:   3,
				Options: []store.ComponentOption{
					{Label: "Bronze Sword", Value: "101", Default: true},
					{Label: "Iron Helm", Value: "102"},
					{Label: "Silk Robe", Value: "103"},
				},
			}},
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(90, 9)
	view.Draw(buf.Clip(buf.Bounds()))

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("multi-select opener was not handled")
	}
	if action, ok := view.TakeComponentAction(); ok {
		t.Fatalf("opening multi-select should not submit, got %+v", action)
	}
	view.Draw(buf.Clip(buf.Bounds()))
	all := rowsText(buf)
	for _, want := range []string{"☑ 2 Bronze Sword", "☐ 3 Iron Helm", "☐ 4 Silk Robe"} {
		if !strings.Contains(all, want) {
			t.Fatalf("expanded multi-select missing %q:\n%s", want, all)
		}
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '3'}) {
		t.Fatal("multi-select option toggle was not handled")
	}
	if action, ok := view.TakeComponentAction(); ok {
		t.Fatalf("toggling multi-select option should not submit, got %+v", action)
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("enter should submit active multi-select")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.CustomID != "sell_items" || action.Label != "Choose one or more items to sell" {
		t.Fatalf("multi-select submit action = %+v,%v", action, ok)
	}
	if got, want := strings.Join(action.Values, ","), "101,102"; got != want {
		t.Fatalf("multi-select values = %q, want %q", got, want)
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("multi-select reopen was not handled")
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("multi-select refold was not handled")
	}
	action, ok = view.TakeComponentAction()
	if !ok || strings.Join(action.Values, ",") != "101,102" {
		t.Fatalf("multi-select refold action = %+v,%v", action, ok)
	}
}

func TestChatViewShiftClickTurnsSingleSelectIntoMultiSelect(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        25,
		ChannelID: 1,
		Author:    "shop",
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{{
				// Incoming selects lose min/max_values on the wire, so this
				// renders as a single-select even though the bot allows many.
				Kind:        store.ComponentSelect,
				Placeholder: "Choose one or more items to sell",
				CustomID:    "sell_items",
				Options: []store.ComponentOption{
					{Label: "Bronze Sword", Value: "101"},
					{Label: "Iron Helm", Value: "102"},
					{Label: "Silk Robe", Value: "103"},
				},
			}},
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(90, 8)
	view.Draw(buf.Clip(buf.Bounds()))

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("select opener was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	optionRow := func(label string) int {
		for y := range buf.Height() {
			if strings.Contains(rowText(buf, y), label) {
				return y
			}
		}
		t.Fatalf("option %q not rendered:\n%s", label, rowsText(buf))
		return -1
	}
	if !strings.Contains(rowsText(buf), "• 2 Bronze Sword") {
		t.Fatalf("single select should render bullets:\n%s", rowsText(buf))
	}

	click := func(label string, shift bool) {
		var mods input.Mod
		if shift {
			mods = input.Shift
		}
		ev := input.MouseEvent{X: 3, Y: optionRow(label), Btn: input.ButtonLeft, Kind: input.MousePress, Mods: mods}
		if !view.Handle(ev) {
			t.Fatalf("click on %q was not handled", label)
		}
		view.Draw(buf.Clip(buf.Bounds()))
	}

	click("Bronze Sword", true)
	if action, ok := view.TakeComponentAction(); ok {
		t.Fatalf("shift-click should not submit, got %+v", action)
	}
	for _, want := range []string{"☑ 2 Bronze Sword", "☐ 3 Iron Helm", "☐ 4 Silk Robe"} {
		if !strings.Contains(rowsText(buf), want) {
			t.Fatalf("multi mode missing %q:\n%s", want, rowsText(buf))
		}
	}

	// Once in multi mode, a plain click toggles instead of submitting.
	click("Iron Helm", false)
	if action, ok := view.TakeComponentAction(); ok {
		t.Fatalf("toggle in multi mode should not submit, got %+v", action)
	}

	// Refolding the control submits every checked value.
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("select refold was not handled")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.CustomID != "sell_items" {
		t.Fatalf("refold action = %+v,%v", action, ok)
	}
	if got, want := strings.Join(action.Values, ","), "101,102"; got != want {
		t.Fatalf("multi values = %q, want %q", got, want)
	}

	// Multi mode is cleared after submit: reopened select is single again.
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("select reopen was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if !strings.Contains(rowsText(buf), "● 2 Bronze Sword") {
		t.Fatalf("reopened select should render bullets again:\n%s", rowsText(buf))
	}
	click("Silk Robe", false)
	action, ok = view.TakeComponentAction()
	if !ok || action.Value != "103" {
		t.Fatalf("plain click after submit should single-select, got %+v,%v", action, ok)
	}
}

func TestChatViewRendersEmbedsWithBorderAndTint(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1,
		Author:    "bot",
		Embeds: []store.Embed{{
			Color:       0x5865F2,
			Title:       "Realm Card",
			Description: "Qi ready",
		}},
	})
	base := screen.Style{Fg: screen.RGB(220, 220, 220), Bg: screen.RGB(10, 10, 10)}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{Text: base})
	buf := screen.NewBuffer(30, 5)
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 1); !strings.HasPrefix(got, "╭") || !strings.HasSuffix(got, "╮") {
		t.Fatalf("embed top border = %q", got)
	}
	if got := rowText(buf, 2); !strings.Contains(got, "│Realm Card") {
		t.Fatalf("embed content row = %q", got)
	}
	border := buf.Cell(0, 1).Style
	content := buf.Cell(1, 2).Style
	if !border.Bg.Set() || !content.Bg.Set() {
		t.Fatalf("embed bg not set: border=%+v content=%+v", border, content)
	}
	if border.Bg == base.Bg || border.Bg == rgbColor(0x5865F2) {
		t.Fatalf("embed bg = %+v, want subtle tint between base and accent", border.Bg)
	}
}

func TestChatViewComponentShortcutsSupportFrenchKeyboardAndZero(t *testing.T) {
	st := store.New(0)
	var children []store.ComponentNode
	for i := 1; i <= 10; i++ {
		children = append(children, store.ComponentNode{
			Kind:     store.ComponentButton,
			Label:    "Action",
			CustomID: "action-" + string(rune('0'+i)),
		})
	}
	children[9].CustomID = "action-10"
	st.AppendMessage(store.Message{
		ID:        22,
		ChannelID: 1,
		Author:    "dao",
		ComponentTree: []store.ComponentNode{{
			Kind:     store.ComponentActionRow,
			Children: children,
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(80, 8)
	view.Draw(buf.Clip(buf.Bounds()))

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'é'}) {
		t.Fatal("AZERTY é shortcut should activate visible shortcut 2")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.CustomID != "action-2" {
		t.Fatalf("AZERTY action = %+v,%v, want action-2", action, ok)
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'à'}) {
		t.Fatal("AZERTY à shortcut should activate visible shortcut 0")
	}
	action, ok = view.TakeComponentAction()
	if !ok || action.CustomID != "action-10" || action.Shortcut != '0' {
		t.Fatalf("zero action = %+v,%v, want action-10 shortcut 0", action, ok)
	}
}

func TestChatViewComponentMouseActivationThroughApp(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        23,
		ChannelID: 1,
		Author:    "dao",
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{{
				Kind:     store.ComponentButton,
				Label:    "Cultivate",
				CustomID: "cultivate",
			}},
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	app := tui.New()
	buf := app.Render(view, tui.Size{W: 40, H: 3})
	if got := rowsText(buf); !strings.Contains(got, "1 Cultivate") {
		t.Fatalf("render did not expose button:\n%s", got)
	}

	if !app.Handle(input.MouseEvent{X: 3, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("app-routed mouse press did not activate component")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.CustomID != "cultivate" {
		t.Fatalf("mouse action = %+v,%v, want cultivate", action, ok)
	}
}

func TestChatViewStylesResolvedMentions(t *testing.T) {
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "hi <@42>"})

	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	accent := screen.Style{Fg: screen.RGB(1, 2, 3), Attrs: screen.Bold}
	view := NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{Accent: accent})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if got := buf.Cell(3, 1).Style; got.Fg != accent.Fg || got.Attrs&screen.Bold == 0 {
		t.Fatalf("mention style = %+v, want accent %+v", got, accent)
	}
}

func rowsText(buf *screen.Buffer) string {
	var b strings.Builder
	for y := 0; y < buf.Height(); y++ {
		b.WriteString(rowText(buf, y))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestChatViewEmptyChannel(t *testing.T) {
	st := store.New(0)
	view := NewChatView(st, func() store.ChannelID { return 99 }, nil, Styles{})
	buf := screen.NewBuffer(10, 3)
	// Should not panic and should render blank rows.
	view.Draw(buf.Clip(buf.Bounds()))
	if rowText(buf, 0) != "" {
		t.Errorf("empty channel row = %q, want blank", rowText(buf, 0))
	}
}

func TestChatViewMouseWheelScrollsMessages(t *testing.T) {
	st := store.New(0)
	for i := 0; i < 6; i++ {
		st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Content: "line"})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	if !view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp}) {
		t.Fatal("wheel up was not handled")
	}
	if view.scroll != 1 {
		t.Fatalf("scroll = %d, want 1", view.scroll)
	}
	if !view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelDown}) {
		t.Fatal("wheel down was not handled")
	}
	if view.scroll != 0 {
		t.Fatalf("scroll = %d, want 0", view.scroll)
	}
}

func TestChatViewRightClickCapturesMessage(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 11, ChannelID: 1, Author: "alice", Content: "line"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if view.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonRight, Y: 1}) {
		t.Fatal("right-click should bubble to Shell for menu anchoring")
	}
	msg, ok := view.TakeContextMessage()
	if !ok || msg.ID != 11 {
		t.Fatalf("context message = %+v,%v, want message 11", msg, ok)
	}
	if _, ok := view.TakeContextMessage(); ok {
		t.Fatal("context message was not cleared after take")
	}
}
