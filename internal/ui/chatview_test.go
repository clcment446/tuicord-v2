package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
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

func TestChatViewRendersConsecutiveMessagesFromAuthorAsOneBlock(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 7, Author: "alice", Content: "first"})
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 7, Author: "alice", Content: "second"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	want := []string{"alice", "first", "second"}
	for i, row := range want {
		if got := rowText(buf, i); got != row {
			t.Errorf("row %d = %q, want %q", i, got, row)
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

func TestChatViewRendersSmallMarkupWithSmallStyle(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{
		Cells: map[string]screen.Style{
			"messages.small": {Fg: screen.RGB(1, 2, 3)},
		},
	})
	lines := view.renderContent("-# small title", 80, screen.Style{})
	if len(lines) != 1 || len(lines[0].segments) == 0 {
		t.Fatalf("small lines = %+v, want one styled line", lines)
	}
	if lines[0].segments[0].style.Fg != screen.RGB(1, 2, 3) {
		t.Fatalf("small style = %+v, want configured small color", lines[0].segments[0].style)
	}
}

func TestChatViewHeadersHaveLevelStylesAndCollapse(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 7, ChannelID: 1, Author: "alice", Content: "# one\ninside one\n## two\ninside two"})
	styles := Styles{Cells: map[string]screen.Style{
		"messages.header1": {Fg: screen.RGB(1, 2, 3), Attrs: screen.Bold},
		"messages.header2": {Fg: screen.RGB(4, 5, 6), Attrs: screen.Underline},
	}}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, styles)
	buf := screen.NewBuffer(40, 6)
	view.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(2, 1).Style.Fg; got != screen.RGB(1, 2, 3) {
		t.Fatalf("h1 color = %+v, want h1 style", got)
	}
	if got := buf.Cell(2, 4).Style.Fg; got != screen.RGB(4, 5, 6) {
		t.Fatalf("h2 color = %+v, want h2 style", got)
	}
	headerY := -1
	for y, line := range view.visibleLines {
		if line.header != nil && line.header.level == 2 {
			headerY = y
			break
		}
	}
	if headerY < 0 || !view.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: 0, Y: headerY}) {
		t.Fatal("header collapse toggle was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if !strings.Contains(rowsText(buf), "inside one") {
		t.Fatal("collapsing h2 should preserve the preceding h1 body")
	}
	if strings.Contains(rowsText(buf), "inside two") {
		t.Fatal("collapsed h2 body remained visible")
	}
}

func TestChatViewMarkupPreservesBackgroundForQuoteAndEmoji(t *testing.T) {
	base := screen.Style{Fg: screen.RGB(220, 220, 220), Bg: screen.RGB(40, 20, 10)}
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{Muted: screen.Style{Fg: screen.RGB(150, 150, 150), Bg: screen.RGB(1, 2, 3)}})
	lines := view.renderContent("> green\n<:wave:123>", 80, base)
	if len(lines) != 2 {
		t.Fatalf("rendered lines = %d, want 2", len(lines))
	}
	for i, line := range lines {
		if len(line.segments) == 0 || line.segments[0].style.Bg != base.Bg {
			t.Errorf("line %d background = %+v, want %+v", i, line.segments[0].style.Bg, base.Bg)
		}
	}
	emojiStyle := view.markupStyle(markup.Span{Kind: markup.Kind_Emoji, Quoted: true}, base)
	if emojiStyle.Bg != base.Bg {
		t.Fatalf("quoted emoji background = %+v, want %+v", emojiStyle.Bg, base.Bg)
	}
}

func TestChatViewRendersMarkedFakeNitroMedia(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	emoji := view.renderContent("[emoji_spin](https://cdn.discordapp.com/emojis/7.gif?size=48&name=spin)", 80, screen.Style{})
	if len(emoji) != 1 || len(emoji[0].segments) != 1 || emoji[0].segments[0].text != "  " {
		t.Fatalf("fake emoji render = %+v, want a two-column inline-media placeholder", emoji)
	}

	sticker := view.renderContent("[sticker_hello](https://media.discordapp.net/stickers/42.png)", 80, screen.Style{})
	if len(sticker) != 1 || len(sticker[0].segments) != 1 || sticker[0].segments[0].text != "[sticker: hello]" {
		t.Fatalf("fake sticker render = %+v, want a sticker media fallback", sticker)
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

func TestChatViewKeepsLoadedEmbedImageInsideTintedFrame(t *testing.T) {
	const imageURL = "https://example.com/card.png"
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        25,
		ChannelID: 1,
		Author:    "bot",
		Embeds: []store.Embed{{
			Kind:     store.EmbedImage,
			ImageURL: imageURL,
			Title:    "Card image",
		}},
	})
	base := screen.Style{Fg: screen.RGB(220, 220, 220), Bg: screen.RGB(10, 10, 10)}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{Text: base})
	view.media = map[string]*chatMediaState{
		imageURL: {img: func() image.Image {
			img := image.NewRGBA(image.Rect(0, 0, 4, 4))
			for y := 0; y < 4; y++ {
				for x := 0; x < 4; x++ {
					img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
				}
			}
			return img
		}()},
	}
	buf := screen.NewBuffer(30, 8)
	view.Draw(buf.Clip(buf.Bounds()))

	for _, row := range []int{5, 6} {
		if left, right := buf.Cell(0, row).Content, buf.Cell(29, row).Content; left != "│" || right != "│" {
			t.Fatalf("image row %d borders = %q/%q, want frame borders; rows:\n%s", row, left, right, rowsText(buf))
		}
		if got := buf.Cell(0, row).Style; !got.Bg.Set() {
			t.Fatalf("left border row %d has no background: %+v", row, got)
		}
		if got := buf.Cell(1, row).Style; !got.Bg.Set() {
			t.Fatalf("image fill row %d has no background: %+v", row, got)
		}
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
	if got := view.bottomScroll.Offset(); got != 1 {
		t.Fatalf("scroll = %d, want 1", got)
	}
	if !view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelDown}) {
		t.Fatal("wheel down was not handled")
	}
	if got := view.bottomScroll.Offset(); got != 0 {
		t.Fatalf("scroll = %d, want 0", got)
	}
}

func TestChatViewEscapeReturnsToNewestMessages(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})

	view.Handle(input.KeyEvent{Key: input.KeyUp})
	view.Handle(input.KeyEvent{Key: input.KeyUp})
	if got := view.bottomScroll.Offset(); got != 2 {
		t.Fatalf("scroll = %d, want 2 before escape", got)
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyEsc}) {
		t.Fatal("escape was not handled")
	}
	if got := view.bottomScroll.Offset(); got != 0 {
		t.Fatalf("scroll = %d after escape, want 0", got)
	}
}

func TestChatViewEscapeKeepsNewestRowVisibleWithStickyAuthor(t *testing.T) {
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 1, Name: "general"})
	st.AppendMessage(store.Message{
		ID:        1,
		ChannelID: 1,
		AuthorID:  42,
		Author:    "alice",
		Content:   "one two three four five six seven eight nine ten eleven twelve",
	})
	st.AppendMessage(store.Message{
		ID:        2,
		ChannelID: 1,
		AuthorID:  43,
		Author:    "bob",
		Content:   "newest-row",
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 4)

	view.Draw(buf.Clip(buf.Bounds()))
	view.Handle(input.KeyEvent{Key: input.KeyUp})
	view.Handle(input.KeyEvent{Key: input.KeyEsc})
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 0); got != "alice" {
		t.Fatalf("sticky author row = %q, want alice", got)
	}
	if got := rowsText(buf); !strings.Contains(got, "newest-row") {
		t.Fatalf("newest row disappeared after returning to bottom:\n%s", got)
	}
}

func TestChatViewOneRowViewportPrioritizesNewestContent(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID:        1,
		ChannelID: 1,
		AuthorID:  42,
		Author:    "alice",
		Content:   "older words wrap across rows newest-row",
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 1)

	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 0); !strings.Contains(got, "newest-row") {
		t.Fatalf("one-row viewport = %q, want newest content", got)
	}
}

func TestChatViewKeepsReadingPositionWhenNewMessageArrives(t *testing.T) {
	st := store.New(0)
	for i := 0; i < 6; i++ {
		st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Content: fmt.Sprintf("message-%d", i)})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 4)

	view.Draw(buf.Clip(buf.Bounds()))
	if !view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp}) {
		t.Fatal("wheel up was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	want := rowsText(buf)

	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Content: "new message"})
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowsText(buf); got != want {
		t.Fatalf("reading viewport moved after new message:\n got %q\nwant %q", got, want)
	}
}

func TestChatViewKeepsReadingPositionWhenOlderMessagesArePrepended(t *testing.T) {
	st := store.New(0)
	for i := 0; i < 6; i++ {
		st.AppendMessage(store.Message{ID: store.MessageID(i + 10), ChannelID: 1, Author: "alice", Content: fmt.Sprintf("message-%d", i)})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 4)

	view.Draw(buf.Clip(buf.Bounds()))
	if !view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp}) {
		t.Fatal("wheel up was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	want := rowsText(buf)

	st.PrependMessages(1, []store.Message{
		{ID: 8, ChannelID: 1, Author: "alice", Content: "older-0"},
		{ID: 9, ChannelID: 1, Author: "alice", Content: "older-1"},
	})
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowsText(buf); got != want {
		t.Fatalf("reading viewport moved after older history was prepended:\n got %q\nwant %q", got, want)
	}
}

func TestChatViewPinsAuthorWhenViewportStartsInsideLongMessage(t *testing.T) {
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 1, Name: "general"})
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, AuthorID: 42, Author: "alice", Content: "one two three four five six seven eight"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(12, 3)

	view.Draw(buf.Clip(buf.Bounds()))
	view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp})
	view.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp})
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 0); got != "alice" {
		t.Fatalf("sticky author row = %q, want alice", got)
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
