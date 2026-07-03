package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
)

func TestKeyMatches(t *testing.T) {
	tests := []struct {
		ev   input.KeyEvent
		spec string
		want bool
	}{
		{input.KeyEvent{Key: input.KeyRune, Rune: 'k', Mods: input.Ctrl}, "ctrl+k", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'k'}, "ctrl+k", false},              // missing ctrl
		{input.KeyEvent{Key: input.KeyRune, Rune: 'k', Mods: input.Ctrl}, "k", false}, // unwanted ctrl
		{input.KeyEvent{Key: input.KeyEsc}, "esc", true},
		{input.KeyEvent{Key: input.KeyTab}, "tab", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'k', Mods: input.Ctrl, Release: true}, "ctrl+k", false},
	}
	for _, tt := range tests {
		if got := keyMatches(tt.ev, tt.spec); got != tt.want {
			t.Errorf("keyMatches(%+v, %q) = %v, want %v", tt.ev, tt.spec, got, tt.want)
		}
	}
}

func TestUnreadBadge(t *testing.T) {
	cases := map[int]string{0: "", -1: "", 3: "3", 99: "99", 100: "99+"}
	for n, want := range cases {
		if got := unreadBadge(n); got != want {
			t.Errorf("unreadBadge(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestQuickSwitcherFilterAndPick(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 1, Name: "gophers"})
	st.UpsertChannel(store.Channel{ID: 10, GuildID: 1, Name: "general", Kind: store.ChannelText})
	st.UpsertChannel(store.Channel{ID: 11, GuildID: 1, Name: "random", Kind: store.ChannelText})
	st.UpsertChannel(store.Channel{ID: 12, GuildID: 1, Name: "voice", Kind: store.ChannelVoice})

	var picked store.ChannelID
	closed := false
	qs := NewQuickSwitcher(st, Styles{},
		func(_ store.GuildID, ch store.ChannelID) { picked = ch },
		func() { closed = true },
	)

	// Voice channels are excluded.
	if len(qs.filtered) != 2 {
		t.Fatalf("filtered = %d entries, want 2 (text only)", len(qs.filtered))
	}

	qs.applyFilter("rand")
	if len(qs.filtered) != 1 || qs.filtered[0].channel != 11 {
		t.Fatalf("filter 'rand' = %+v, want only #random", qs.filtered)
	}

	qs.pick()
	if picked != 11 {
		t.Errorf("picked = %d, want 11", picked)
	}
	if !closed {
		t.Error("pick did not close the switcher")
	}
}

func TestShellTogglesOverlays(t *testing.T) {
	cfg := config.Default()
	sh := &Shell{cfg: cfg}

	// Open quick switcher directly (no app needed for toggle state).
	sh.overlay = NewHelpOverlay(cfg)
	if sh.current() == nil || sh.overlay == nil {
		t.Fatal("help overlay not set")
	}

	// Esc closes the overlay.
	handled := sh.Handle(input.KeyEvent{Key: input.KeyEsc})
	if !handled || sh.overlay != nil {
		t.Errorf("Esc did not close overlay (handled=%v, overlay=%v)", handled, sh.overlay)
	}
}
