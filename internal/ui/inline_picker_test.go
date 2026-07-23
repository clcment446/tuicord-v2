package ui

import (
	"fmt"
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
)

func TestInlinePickerEmojiFuzzyMatchAndFakeNitroInsert(t *testing.T) {
	st := newTestPickerStore()
	st.SetGuildEmojis(2, []store.GuildEmoji{{ID: 20, Name: "partyBlob", Animated: true}})

	var inserted string
	p := NewInlinePicker(st, Styles{}, 1, 0, false, true, ':', "ptb",
		func(s string) { inserted = s }, nil, func() {})
	if len(p.filtered) == 0 || !strings.Contains(p.filtered[0].label, "partyBlob") {
		t.Fatalf("fuzzy emoji results = %+v", p.filtered)
	}
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	want := "[emoji_partyBlob](https://cdn.discordapp.com/emojis/20.gif?size=48&name=partyBlob)"
	if inserted != want {
		t.Fatalf("inserted = %q, want %q", inserted, want)
	}
}

func TestInlinePickerCapsLargeCatalog(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	emojis := make([]store.GuildEmoji, 500)
	for i := range emojis {
		emojis[i] = store.GuildEmoji{ID: uint64(i + 1), Name: fmt.Sprintf("blob-%03d", i)}
	}
	st.SetGuildEmojis(1, emojis)
	p := NewInlinePicker(st, Styles{}, 1, 0, false, true, ':', "blob", func(string) {}, nil, func() {})
	if len(p.filtered) != maxPickerResults {
		t.Fatalf("results = %d, want cap %d", len(p.filtered), maxPickerResults)
	}
}

func TestInlinePickerRefreshPreservesSelection(t *testing.T) {
	p := NewInlinePicker(newTestPickerStore(), Styles{}, 1, 0, false, true, ':', "", func(string) {}, nil, func() {})
	p.list.SetSelectedSilent(2)
	p.refilter()
	if got := p.list.Selected(); got != 2 {
		t.Fatalf("selected after refresh = %d, want 2", got)
	}
}

func TestInlinePickerMentionForms(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general"})
	st.UpsertMember(1, store.Member{ID: 20, Name: "Ada Lovelace"})
	st.UpsertRole(1, store.Role{ID: 30, Name: "Engineering", Position: 1})

	tests := []struct {
		trigger rune
		query   string
		want    string
	}{
		{'#', "gnrl", "<#10>"},
		{'@', "adl", "<@20>"},
		{'&', "eng", "<@&30>"},
	}
	for _, tt := range tests {
		t.Run(string(tt.trigger), func(t *testing.T) {
			var inserted string
			p := NewInlinePicker(st, Styles{}, 1, 0, false, true, tt.trigger, tt.query,
				func(s string) { inserted = s }, nil, func() {})
			p.Handle(input.KeyEvent{Key: input.KeyEnter})
			if inserted != tt.want {
				t.Fatalf("inserted = %q, want %q", inserted, tt.want)
			}
		})
	}
}

func TestInlinePickerNativeStickerSelectsSticker(t *testing.T) {
	st := newTestPickerStore()
	var selected uint64
	p := NewInlinePicker(st, Styles{}, 1, 0, false, true, '%', "hel", func(string) {},
		func(id uint64) { selected = id }, func() {})
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if selected != 99 {
		t.Fatalf("selected sticker = %d, want 99", selected)
	}
}

func TestInlinePickerOrdersCustomEmojiByFavoriteThenActiveGuild(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertGuild(store.Guild{ID: 2, Name: "Other"})
	st.SetGuildEmojis(1, []store.GuildEmoji{{ID: 10, Name: "blob-local"}})
	st.SetGuildEmojis(2, []store.GuildEmoji{{ID: 20, Name: "blob-favorite"}, {ID: 30, Name: "blob-other"}})
	p := NewInlinePicker(st, Styles{}, 1, 0, false, true, ':', "blob", func(string) {}, nil, func() {})
	p.SetFavorites([]string{"e:20"}, nil)

	got := []string{p.filtered[0].label, p.filtered[1].label, p.filtered[2].label}
	want := []string{":blob-favorite:  (fake-nitro)", ":blob-local:", ":blob-other:  (fake-nitro)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("emoji order = %v, want %v", got, want)
	}
}

func TestInlinePickerOrdersStickersByFavoriteThenActiveGuild(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Home"})
	st.UpsertGuild(store.Guild{ID: 2, Name: "Other"})
	st.SetGuildStickers(1, []store.GuildSticker{{ID: 10, Name: "blob-local"}})
	st.SetGuildStickers(2, []store.GuildSticker{{ID: 20, Name: "blob-favorite"}, {ID: 30, Name: "blob-other"}})
	p := NewInlinePicker(st, Styles{}, 1, 0, false, true, '%', "blob", func(string) {}, nil, func() {})
	p.SetFavorites(nil, []uint64{20})

	got := []string{p.filtered[0].label, p.filtered[1].label, p.filtered[2].label}
	want := []string{"blob-favorite  (fake-nitro)", "blob-local  (native)", "blob-other  (fake-nitro)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sticker order = %v, want %v", got, want)
	}
}

func TestCompletionToken(t *testing.T) {
	tests := []struct {
		value       string
		wantTrigger rune
		wantStart   int
		wantQuery   string
		wantOK      bool
	}{
		{value: "hello :ptb", wantTrigger: ':', wantStart: len("hello "), wantQuery: "ptb", wantOK: true},
		{value: "#general", wantTrigger: '#', wantStart: 0, wantQuery: "general", wantOK: true},
		{value: ";sett", wantTrigger: ';', wantStart: 0, wantQuery: "sett", wantOK: true},
		{value: "hello world", wantOK: false},
		{value: "hello :two words", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			trigger, start, query, ok := completionToken(tt.value, len(tt.value))
			if trigger != tt.wantTrigger || start != tt.wantStart || query != tt.wantQuery || ok != tt.wantOK {
				t.Fatalf("completionToken(%q) = %q,%d,%q,%v", tt.value, trigger, start, query, ok)
			}
		})
	}
}

func TestInlinePickerBackspaceAtEmptyRemovesTrigger(t *testing.T) {
	removed := false
	closed := false
	p := NewInlinePicker(newTestPickerStore(), Styles{}, 1, 0, false, true, '&', "", func(string) {}, nil, func() { closed = true })
	p.SetTriggerDelete(func() { removed = true })
	p.Handle(input.KeyEvent{Key: input.KeyBackspace})
	if !removed || !closed {
		t.Fatalf("backspace removed=%v closed=%v, want both true", removed, closed)
	}
}

func TestFuzzyScoreTreatsStarAsWildcard(t *testing.T) {
	if _, ok := fuzzyScore("cult-of-the-green", "cult*gr*"); !ok {
		t.Fatal("star wildcard did not match ordered fragments")
	}
	if _, ok := fuzzyScore("green-cult", "cult*gr"); ok {
		t.Fatal("wildcard must preserve fragment order")
	}
}

func TestInlinePickerMentionUsesActiveDMRecipients(t *testing.T) {
	const dmGuild = ^store.GuildID(0)
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: dmGuild, Name: "Direct Messages"})
	st.UpsertChannel(store.Channel{ID: 90, GuildID: dmGuild, Name: "Ada", Kind: store.ChannelDM,
		Recipients: []store.Member{{ID: 20, Name: "Ada Lovelace"}}})

	var inserted string
	p := NewInlinePicker(st, Styles{}, dmGuild, 90, false, true, '@', "adl",
		func(s string) { inserted = s }, nil, func() {})
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if inserted != "<@20>" {
		t.Fatalf("DM mention inserted = %q, want recipient mention", inserted)
	}
}

func TestMemberForContextResolvesSentDMValue(t *testing.T) {
	const dmGuild = ^store.GuildID(0)
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 90, GuildID: dmGuild, Kind: store.ChannelDM,
		Recipients: []store.Member{{ID: 20, Name: "Ada Lovelace"}}})

	m, ok := memberForContext(st, dmGuild, 90, 20, store.Member{}, false)
	if !ok || m.Name != "Ada Lovelace" {
		t.Fatalf("DM mention resolution = %+v,%v, want Ada Lovelace", m, ok)
	}
}

func TestMemberForContextResolvesSelfMention(t *testing.T) {
	const dmGuild = ^store.GuildID(0)
	st := store.New(0)
	// A DM channel: recipients exclude the logged-in user, so a self-mention
	// only resolves through the self fallback.
	st.UpsertChannel(store.Channel{ID: 90, GuildID: dmGuild, Kind: store.ChannelDM,
		Recipients: []store.Member{{ID: 20, Name: "Ada Lovelace"}}})
	self := store.Member{ID: 7, Name: "Me"}

	m, ok := memberForContext(st, dmGuild, 90, 7, self, true)
	if !ok || m.Name != "Me" {
		t.Fatalf("self mention resolution = %+v,%v, want Me", m, ok)
	}

	// Without a known self identity it stays unresolved.
	if _, ok := memberForContext(st, dmGuild, 90, 7, store.Member{}, false); ok {
		t.Fatal("self mention resolved without a known self identity")
	}
}

func TestHotSwitchStructuredServerAndChannelSearch(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "Cult of the Green"})
	st.UpsertGuild(store.Guild{ID: 2, Name: "Gaming Lounge"})
	st.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general", Kind: store.ChannelText})
	st.UpsertChannel(store.Channel{ID: 11, GuildID: 1, Name: "photos", Kind: store.ChannelText})
	st.UpsertChannel(store.Channel{ID: 20, GuildID: 2, Name: "general", Kind: store.ChannelText})

	p := NewInlinePicker(st, Styles{}, 1, 10, false, true, '+', `\cult gr#pho`, func(string) {}, nil, func() {})
	if len(p.filtered) != 1 || p.filtered[0].switchChannel != 11 {
		t.Fatalf("structured results = %+v, want Cult/photos", p.filtered)
	}

	// Ordinary search remains a fuzzy match against the full display label.
	p.query = "gaming gen"
	p.refilter()
	if len(p.filtered) != 1 || p.filtered[0].switchChannel != 20 {
		t.Fatalf("ordinary results = %+v, want Gaming/general", p.filtered)
	}
}
