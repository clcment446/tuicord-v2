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
	uitext "awesomeProject/internal/tui/text"
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
	headerY := -1
	for y := 0; y < buf.Height(); y++ {
		if strings.HasPrefix(rowText(buf, y), "▾ two") {
			headerY = y
			break
		}
	}
	if got := buf.Cell(2, 1).Style.Fg; got != screen.RGB(1, 2, 3) {
		t.Fatalf("h1 color = %+v, want h1 style", got)
	}
	if headerY < 0 || buf.Cell(2, headerY).Style.Fg != screen.RGB(4, 5, 6) {
		t.Fatalf("h2 color = %+v, want h2 style", buf.Cell(2, headerY).Style.Fg)
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

func TestChatViewHeaderHitMatchesVisibleHeaderRow(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 7, ChannelID: 1, Author: "alice", Content: "# title\nbody"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 1); got != "▾ title" {
		t.Fatalf("header row = %q, want visible header marker", got)
	}
	if !view.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, X: 0, Y: 1}) {
		t.Fatal("clicking visible header marker was not handled")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if strings.Contains(rowsText(buf), "body") {
		t.Fatal("header body remained visible after clicking its marker")
	}
}

func TestPrependComponentFramePreservesAndOffsetsInteractionMetadata(t *testing.T) {
	header := &headerHit{key: "heading", level: 1}
	line := chatLine{
		segments: []chatSegment{{text: "hello"}},
		entities: []entityHit{{start: 0, end: 2, action: markup.Action{Kind: markup.ActionUserMention, Target: "7"}}},
		header:   header,
		spinner:  true,
	}

	got := prependComponentFrame(line, componentFrame{prefix: "│ "})
	if len(got.entities) != 1 || got.entities[0].start != 2 || got.entities[0].end != 4 {
		t.Fatalf("entity hits = %+v, want offset hit [2,4)", got.entities)
	}
	if got.header != header || !got.spinner {
		t.Fatalf("framed metadata = header %p spinner %t, want preserved", got.header, got.spinner)
	}
}

func TestChatViewComponentControlsStayWithinNarrowWidth(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	message := store.Message{ID: 1, ChannelID: 1}
	lines := view.renderComponentControls(&componentRenderContext{}, message, []store.ComponentNode{{
		Kind: store.ComponentButton, CustomID: "wide", Label: "界界界界界界",
	}}, 8, screen.Style{}, componentFrame{})

	if len(lines) != 1 {
		t.Fatalf("control lines = %d, want one truncated line", len(lines))
	}
	if got := uitext.Width(lineText(lines[0])); got > 8 {
		t.Fatalf("control width = %d, want <= 8: %q", got, lineText(lines[0]))
	}
	if len(lines[0].actions) != 1 || lines[0].actions[0].end > 8 {
		t.Fatalf("control hits = %+v, want one visible bounded hit", lines[0].actions)
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

func TestChatViewComponentShortcutsAreScopedToFocusedMessage(t *testing.T) {
	st := store.New(0)
	for id, label := range []string{"older", "newer"} {
		st.AppendMessage(store.Message{
			ID: store.MessageID(id + 1), ChannelID: 1, Author: label, Content: label,
			ComponentTree: []store.ComponentNode{{Kind: store.ComponentButton, CustomID: label, Label: label}},
		})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(50, 8)
	view.Draw(buf.Clip(buf.Bounds()))
	view.Draw(buf.Clip(buf.Bounds()))

	if got := strings.Count(rowsText(buf), "1 newer"); got != 1 || strings.Contains(rowsText(buf), "1 older") {
		t.Fatalf("shortcuts were not scoped to newest focused message:\n%s", rowsText(buf))
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("focused message shortcut was not handled")
	}
	action, ok := view.TakeComponentAction()
	if !ok || action.Message.ID != 2 || action.CustomID != "newer" {
		t.Fatalf("action = %+v,%v, want newer message", action, ok)
	}

	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'}) {
		t.Fatal("k did not move to the previous stopping point")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if got := rowsText(buf); !strings.Contains(got, "1 older") || strings.Contains(got, "1 newer") {
		t.Fatalf("shortcut labels did not follow focused message:\n%s", got)
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("older focused message shortcut was not handled")
	}
	action, ok = view.TakeComponentAction()
	if !ok || action.Message.ID != 1 || action.CustomID != "older" {
		t.Fatalf("action = %+v,%v, want older message", action, ok)
	}
}

func TestChatViewEmbedFirstLineIsStopAndAuthorNeverIs(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bot", Embeds: []store.Embed{{Title: "Card", Description: "body"}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(50, 8)
	view.Draw(buf.Clip(buf.Bounds()))

	if len(view.focusStops) != 1 {
		t.Fatalf("focus stops = %+v, want one embed-first-line stop", view.focusStops)
	}
	stop := view.focusStops[0]
	lines := view.render(50)
	if stop.kind != chatFocusMessage || stop.line < 1 || !lines[stop.line].embedStart {
		t.Fatalf("stop = %+v, want embed first line and never author", stop)
	}
}

func TestChatViewMultipleEmbedStopsHaveStableDistinctKeys(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bot", Content: "intro", Embeds: []store.Embed{{Title: "One"}, {Title: "Two"}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(50, 12)
	view.Draw(buf.Clip(buf.Bounds()))
	var embedKeys []string
	for _, stop := range view.focusStops {
		if strings.Contains(stop.key, ":embed:") {
			embedKeys = append(embedKeys, stop.key)
		}
	}
	if len(embedKeys) != 2 || embedKeys[0] == embedKeys[1] {
		t.Fatalf("embed stop keys = %v, want two stable distinct keys", embedKeys)
	}
}

func TestChatViewDashTogglesFocusedHeader(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bot", Content: "# section\ninside"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(40, 5)
	view.Draw(buf.Clip(buf.Bounds()))

	key := input.KeyEvent{Key: input.KeyRune, Rune: '-'}
	if !view.Handle(key) || !view.collapsedHeaders[view.focusStops[0].headerKey] {
		t.Fatal("first - did not collapse header")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if !view.Handle(key) || view.collapsedHeaders[view.focusStops[0].headerKey] {
		t.Fatal("second - did not expand header")
	}
}

func TestChatViewCollapsedH1SkipsNestedSectionsUntilNextH1(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.headerMessageKey = "m"
	view.collapsedHeaders = map[string]bool{"m:header:0": true}
	lines := view.renderContent("# one\nalpha\n## nested\nbeta\n# two\ngamma", 80, screen.Style{})
	var got strings.Builder
	for _, line := range lines {
		for _, segment := range line.segments {
			got.WriteString(segment.text)
		}
		got.WriteByte('\n')
	}
	text := got.String()
	if strings.Contains(text, "alpha") || strings.Contains(text, "nested") || strings.Contains(text, "beta") {
		t.Fatalf("collapsed h1 leaked nested section: %q", text)
	}
	if !strings.Contains(text, "two") || !strings.Contains(text, "gamma") {
		t.Fatalf("next h1 section missing: %q", text)
	}
}

func TestChatViewMouseBreakpointTrackingIsOptIn(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "a", Content: "first"})
	st.AppendMessage(store.Message{ID: 2, ChannelID: 1, Author: "b", Content: "second"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 6)
	view.Draw(buf.Clip(buf.Bounds()))
	before := view.focusStopKey
	view.Handle(input.MouseEvent{Kind: input.MouseMotion, X: 0, Y: 1})
	if view.focusStopKey != before {
		t.Fatalf("default mouse tracking moved focus from %q to %q", before, view.focusStopKey)
	}
	view.SetMouseBreakpointTracking(true)
	view.Handle(input.MouseEvent{Kind: input.MouseMotion, X: 0, Y: 0})
	if view.focusStopKey != before {
		t.Fatal("author line was treated as a breakpoint")
	}
	view.Handle(input.MouseEvent{Kind: input.MouseMotion, X: 0, Y: 1})
	if view.focusStopKey == before {
		t.Fatal("enabled mouse tracking did not move breakpoint focus")
	}
}

func TestChatViewVimKeysAreOptIn(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bot", Content: "one"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 4)
	view.Draw(buf.Clip(buf.Bounds()))
	if view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) {
		t.Fatal("j was handled while Vim navigation was disabled")
	}
	view.SetVimNavigation(true)
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) {
		t.Fatal("j was not handled after enabling Vim navigation")
	}
}

func TestChatViewUDispatchesFocusedAuthorProfileAction(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, AuthorID: 42, Author: "alice", Content: "hello"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 4)
	view.Draw(buf.Clip(buf.Bounds()))
	var action rune
	var author store.UserID
	view.OnMessageAction(func(got rune, msg store.Message) { action, author = got, msg.AuthorID })
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'U'}) {
		t.Fatal("U was not handled")
	}
	if action != 'u' || author != 42 {
		t.Fatalf("profile action = %q for %d, want u for 42", action, author)
	}
}

func TestChatViewWholeBlockHighlightIsOptIn(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "anchor\ncontinued\n## next\noutside"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 6)
	view.Draw(buf.Clip(buf.Bounds()))
	if buf.Cell(0, 2).Style.Attrs&screen.Reverse != 0 {
		t.Fatal("continuation line highlighted while block highlighting was disabled")
	}

	view.SetHighlightFocusBlock(true)
	buf = screen.NewBuffer(30, 6)
	view.Draw(buf.Clip(buf.Bounds()))
	if buf.Cell(0, 2).Style.Attrs&screen.Reverse == 0 {
		t.Fatal("continuation line was not included in focused block")
	}
	if buf.Cell(20, 1).Style.Attrs&screen.Reverse == 0 {
		t.Fatal("focused line padding was not highlighted")
	}
	if buf.Cell(0, 3).Style.Attrs&screen.Reverse != 0 {
		t.Fatal("next stopping point leaked into focused block")
	}
}

func TestChatViewBlockHighlightDoesNotIncludeEmbedBorders(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Embeds: []store.Embed{{Title: "Release", Description: "Ready"}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(true)
	view.SetHighlightFocusBlock(true)
	buf := screen.NewBuffer(30, 6)
	view.Draw(buf.Clip(buf.Bounds()))

	var border, content *screen.Cell
	for y := 0; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			cell := buf.Cell(x, y)
			if cell.Content == "╭" {
				copy := cell
				border = &copy
			}
			if cell.Content == "R" {
				copy := cell
				content = &copy
			}
		}
	}
	if border == nil || content == nil {
		t.Fatalf("embed cells not rendered:\n%s", rowsText(buf))
	}
	if border.Style.Attrs&screen.Reverse != 0 {
		t.Fatal("embed border was included in the focus highlight")
	}
	if content.Style.Attrs&screen.Reverse == 0 {
		t.Fatal("embed contents were not included in the focus highlight")
	}
}

func TestFocusedStyleSwapsByDefaultAndHonorsExplicitColors(t *testing.T) {
	base := screen.Style{Fg: screen.RGB(1, 2, 3), Bg: screen.RGB(4, 5, 6)}
	if got := (Styles{}).focusedStyle(base); got.Attrs&screen.Reverse == 0 {
		t.Fatalf("default focused style = %+v, want reverse video", got)
	}
	styles := Styles{Cells: map[string]screen.Style{
		"messages.focused": {Fg: screen.RGB(7, 8, 9), Bg: screen.RGB(10, 11, 12), Attrs: screen.Reverse},
	}}
	if got := styles.focusedStyle(base); got.Fg != screen.RGB(7, 8, 9) || got.Bg != screen.RGB(10, 11, 12) || got.Attrs&screen.Reverse != 0 {
		t.Fatalf("explicit focused style = %+v, want un-reversed configured colors", got)
	}
}

func TestChatViewVimMultiSelectCopiesSelectedMessages(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "first"})
	st.AppendMessage(store.Message{ID: 2, ChannelID: 1, Author: "bob", Content: "second"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 6)
	view.Draw(buf.Clip(buf.Bounds()))

	var copied []store.Message
	view.OnMessageCopy(func(messages []store.Message) { copied = append([]store.Message(nil), messages...) })
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'V', Mods: input.Shift}) {
		t.Fatal("V did not select the focused message")
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'}) {
		t.Fatal("k did not move to the previous message")
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'Y', Mods: input.Shift}) {
		t.Fatal("Y did not copy selected messages")
	}
	if len(copied) != 2 || copied[0].Content != "first" || copied[1].Content != "second" {
		t.Fatalf("copied = %#v, want messages in chronological order", copied)
	}
}

func TestChatViewVimYCopiesFocusedMessageWithoutSelection(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "only"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(30, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	var copied []store.Message
	view.OnMessageCopy(func(messages []store.Message) { copied = append([]store.Message(nil), messages...) })
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'Y', Mods: input.Shift}) {
		t.Fatal("Y did not copy the focused message")
	}
	if len(copied) != 1 || copied[0].Content != "only" {
		t.Fatalf("copied = %#v, want focused message", copied)
	}
}

func TestChatViewUnfocusedHidesAndRejectsComponentShortcuts(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bot", ComponentTree: []store.ComponentNode{{
		Kind: store.ComponentButton, CustomID: "go", Label: "Go",
	}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetFocusOwner(false)
	buf := screen.NewBuffer(40, 4)
	view.Draw(buf.Clip(buf.Bounds()))
	if strings.Contains(rowsText(buf), "1 Go") {
		t.Fatalf("unfocused shortcut was rendered:\n%s", rowsText(buf))
	}
	if view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '1'}) {
		t.Fatal("unfocused shortcut was handled")
	}
}

func TestChatViewVimStopsIncludeFirstLineHeadersAndEveryControl(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 1, ChannelID: 1, Author: "bot", Content: "# first\nbody\n## second\nmore",
		ComponentTree: []store.ComponentNode{{Kind: store.ComponentActionRow, Children: []store.ComponentNode{
			{Kind: store.ComponentButton, CustomID: "a", Label: "A"},
			{Kind: store.ComponentButton, CustomID: "b", Label: "B"},
		}}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetVimNavigation(true)
	view.SetFocusOwner(true)
	buf := screen.NewBuffer(50, 9)
	view.Draw(buf.Clip(buf.Bounds()))
	view.Draw(buf.Clip(buf.Bounds()))

	if got := len(view.focusStops); got != 4 {
		t.Fatalf("focus stops = %d, want first/header + second header + two controls: %+v", got, view.focusStops)
	}
	if view.focusStops[0].kind != chatFocusHeader || view.focusStops[1].kind != chatFocusHeader ||
		view.focusStops[2].kind != chatFocusControl || view.focusStops[3].kind != chatFocusControl {
		t.Fatalf("focus stop kinds = %+v", view.focusStops)
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) || view.focusStops[view.focusStopIndex].headerKey == "" {
		t.Fatal("j did not move from first header to second header")
	}
	if !view.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '-'}) {
		t.Fatal("- did not fold the focused header")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if strings.Contains(rowsText(buf), "more") {
		t.Fatalf("focused header body remained visible after fold:\n%s", rowsText(buf))
	}
	if !view.HandleVimFocus(true) {
		t.Fatal("l-style focus traversal did not unfold the collapsed header")
	}
	view.Draw(buf.Clip(buf.Bounds()))
	if !strings.Contains(rowsText(buf), "more") {
		t.Fatalf("collapsed header did not unfold:\n%s", rowsText(buf))
	}
}
