package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
)

func newTestPickerStore() *store.Store {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertGuild(store.Guild{ID: 2, Name: "Other"})
	st.SetGuildEmojis(1, []store.GuildEmoji{
		{ID: 10, Name: "homeblob", Animated: false}, // native (same guild, static)
	})
	st.SetGuildEmojis(2, []store.GuildEmoji{
		{ID: 20, Name: "otherspin", Animated: true}, // other guild animated → fake-nitro
	})
	st.SetGuildStickers(1, []store.GuildSticker{{ID: 99, Name: "hello"}})
	return st
}

func typeRunes(p *Picker, s string) {
	for _, r := range s {
		p.Handle(input.KeyEvent{Key: input.KeyRune, Rune: r})
	}
}

func TestPickerEmojiFilter(t *testing.T) {
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	typeRunes(p, "fire")
	if len(p.filtered) == 0 || !strings.Contains(p.filtered[0].label, ":fire:") {
		t.Fatalf("emoji filter 'fire' = %+v", p.filtered)
	}
	if p.filtered[0].insert != "🔥" {
		t.Fatalf("fire insert = %q, want the emoji char", p.filtered[0].insert)
	}
}

func TestPickerCustomTabNativeAndFakeNitro(t *testing.T) {
	// active guild 1, no nitro, fake-nitro on.
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	p.setTab(tabCustom)

	byName := map[string]pickerEntry{}
	for _, e := range p.filtered {
		byName[strings.SplitN(e.label, " ", 2)[0]] = e
	}
	home := byName[":homeblob:"]
	if home.insert != "<:homeblob:10>" {
		t.Fatalf("same-guild static insert = %q, want native mention", home.insert)
	}
	other := byName[":otherspin:"]
	if !strings.HasPrefix(other.insert, "https://cdn.discordapp.com/emojis/20.gif") {
		t.Fatalf("other-guild animated insert = %q, want fake-nitro url", other.insert)
	}
	if !strings.Contains(other.label, "fake-nitro") {
		t.Fatalf("fake-nitro entry not labelled: %q", other.label)
	}
}

func TestPickerCustomLockedWithoutFakeNitro(t *testing.T) {
	// no nitro, fake-nitro off → other-guild animated emoji is locked.
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, false, func(string) {}, func() {})
	p.setTab(tabCustom)
	typeRunes(p, "otherspin")
	if len(p.filtered) != 1 {
		t.Fatalf("expected the one otherspin entry, got %+v", p.filtered)
	}
	if p.filtered[0].usable {
		t.Fatal("other-guild animated emoji should be locked without nitro/fake-nitro")
	}
	if !strings.Contains(p.filtered[0].label, "locked") {
		t.Fatalf("locked entry not labelled: %q", p.filtered[0].label)
	}
}

func TestPickerInsertOnEnter(t *testing.T) {
	var inserted string
	closed := false
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true,
		func(s string) { inserted = s }, func() { closed = true })
	typeRunes(p, "fire")
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if inserted != "🔥" {
		t.Fatalf("inserted = %q, want fire emoji", inserted)
	}
	if !closed {
		t.Fatal("picker did not close after insert")
	}
}

func TestPickerLockedInsertIsNoOp(t *testing.T) {
	inserted := false
	closed := false
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, false,
		func(string) { inserted = true }, func() { closed = true })
	p.setTab(tabCustom)
	typeRunes(p, "otherspin")
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if inserted || closed {
		t.Fatal("picking a locked entry should do nothing")
	}
}

func TestPickerTabSwitchWraps(t *testing.T) {
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	if p.tab != tabEmoji {
		t.Fatal("picker should open on the emoji tab")
	}
	p.Handle(input.KeyEvent{Key: input.KeyLeft}) // wraps to sticker
	if p.tab != tabSticker {
		t.Fatalf("left from emoji = %v, want sticker (wrap)", p.tab)
	}
	p.Handle(input.KeyEvent{Key: input.KeyRight}) // back to emoji
	if p.tab != tabEmoji {
		t.Fatalf("right from sticker = %v, want emoji", p.tab)
	}
}

func TestPickerStickerTab(t *testing.T) {
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	p.setTab(tabSticker)
	if len(p.filtered) != 1 || !strings.HasPrefix(p.filtered[0].label, "hello") {
		t.Fatalf("sticker tab = %+v", p.filtered)
	}
	if p.filtered[0].stickerID != 99 || p.filtered[0].insert != "" {
		t.Fatalf("native sticker entry = %+v", p.filtered[0])
	}
}

func TestPickerNativeStickerSelectionAndRecentOrder(t *testing.T) {
	st := newTestPickerStore()
	st.SetGuildStickers(1, []store.GuildSticker{{ID: 99, Name: "hello"}, {ID: 100, Name: "wave"}})
	var selected uint64
	p := NewPicker(st, Styles{}, 1, false, true, func(string) {}, func() {})
	p.SetStickerSelect(func(id uint64) { selected = id })
	p.SetRecentStickers([]uint64{100})
	p.setTab(tabSticker)
	if len(p.filtered) != 2 || p.filtered[0].stickerID != 100 {
		t.Fatalf("recent sticker order = %+v", p.filtered)
	}
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if selected != 100 {
		t.Fatalf("selected sticker = %d, want 100", selected)
	}
}

func TestPickerOtherGuildStickerUsesFakeNitroURL(t *testing.T) {
	st := newTestPickerStore()
	st.SetGuildStickers(2, []store.GuildSticker{{ID: 200, Name: "other"}})
	p := NewPicker(st, Styles{}, 1, false, true, func(string) {}, func() {})
	p.setTab(tabSticker)
	typeRunes(p, "other")
	if len(p.filtered) != 1 || p.filtered[0].stickerID != 0 ||
		!strings.Contains(p.filtered[0].insert, "/stickers/200.png") {
		t.Fatalf("other-guild sticker = %+v", p.filtered)
	}
}

func TestPickerGIFSearchAndInsert(t *testing.T) {
	var requested string
	var inserted string
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true,
		func(s string) { inserted = s }, func() {})
	p.SetGIFSearch(func(query string, done func([]GIFResult, error)) {
		requested = query
		done([]GIFResult{{Title: "Party Cat", URL: "https://media.tenor.com/party.gif"}}, nil)
	})
	p.setTab(tabGIF)
	typeRunes(p, "party cat")
	if requested != "party cat" {
		t.Fatalf("gif query = %q", requested)
	}
	if len(p.filtered) != 1 || p.filtered[0].insert != "https://media.tenor.com/party.gif" {
		t.Fatalf("gif results = %+v", p.filtered)
	}
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if inserted != "https://media.tenor.com/party.gif" {
		t.Fatalf("inserted = %q", inserted)
	}
}

func TestPickerBackspace(t *testing.T) {
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	typeRunes(p, "fire")
	p.Handle(input.KeyEvent{Key: input.KeyBackspace})
	if p.query != "fir" {
		t.Fatalf("query after backspace = %q, want 'fir'", p.query)
	}
}

func TestPickerRendersTabsAndResults(t *testing.T) {
	p := NewPicker(newTestPickerStore(), Styles{}, 1, false, true, func(string) {}, func() {})
	buf := tui.New().Render(p, tui.Size{W: 60, H: 16})
	if !bufferContains(buf, "Emoji") {
		t.Fatal("tab strip not rendered")
	}
	if !bufferContains(buf, "esc close") {
		t.Fatal("hint line not rendered")
	}
}
