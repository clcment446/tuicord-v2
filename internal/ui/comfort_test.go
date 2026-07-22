package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
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
		{input.KeyEvent{Key: input.KeyRune, Rune: 'n', Mods: input.Ctrl | input.Shift}, "ctrl+n", false},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'n', Mods: input.Ctrl | input.Shift}, "ctrl+shift+n", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'G', Mods: input.Shift}, "G", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'g'}, "G", false},
		{input.KeyEvent{Key: input.KeyTab, Mods: input.Shift}, "tab", false},
		{input.KeyEvent{Key: input.KeyTab, Mods: input.Shift}, "shift+tab", true},
		{input.KeyEvent{Key: input.KeyEsc}, "esc", true},
		{input.KeyEvent{Key: input.KeyTab}, "tab", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: ' '}, "space", true},
		{input.KeyEvent{Key: input.KeyLeft}, "left", true},
		{input.KeyEvent{Key: input.KeyRight}, "right", true},
		{input.KeyEvent{Key: input.KeyRune, Rune: 'k', Mods: input.Ctrl, Release: true}, "ctrl+k", false},
	}
	for _, tt := range tests {
		if got := keyMatches(tt.ev, tt.spec); got != tt.want {
			t.Errorf("keyMatches(%+v, %q) = %v, want %v", tt.ev, tt.spec, got, tt.want)
		}
	}
}

func TestUnreadBadge(t *testing.T) {
	cases := map[int]string{0: "", -1: "", 3: "3", 99: "99", 100: "99"}
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

func TestShellPopupRendersOverMainView(t *testing.T) {
	menu := widget.NewMenu([]widget.MenuItem{{Label: "Reply"}})
	menu.SetAnchor(12, 1)
	sh := &Shell{
		cfg:   config.Default(),
		mv:    &MainView{Root: widget.NewText("main view remains")},
		popup: menu,
	}
	menu.OnDismiss(sh.closePopup)

	buf := tui.New().Render(sh, tui.Size{W: 40, H: 6})
	if !bufferContains(buf, "main view remains") {
		t.Fatal("popup replaced the main view instead of drawing over it")
	}
	if !bufferContains(buf, "Reply") {
		t.Fatal("popup menu was not rendered")
	}
	if !sh.Handle(input.KeyEvent{Key: input.KeyEsc}) || sh.popup != nil {
		t.Fatal("Esc did not dismiss popup menu")
	}
}

func TestToastAndPopupPointerRoutingMatchesOverlayDrawOrder(t *testing.T) {
	underlyingHits := 0
	popupHits := 0
	button := widget.NewButton("covered", func() { underlyingHits++ })
	sh := &Shell{cfg: config.Default(), mv: &MainView{Root: button}}
	menu := widget.NewMenu([]widget.MenuItem{{Label: "Open", OnSelect: func() { popupHits++ }}})
	menu.SetAnchor(20, 6)
	menu.OnDismiss(sh.closePopup)
	sh.popup = menu
	sh.toasts = []*Toast{NewToast("Notice", "opaque", Styles{})}
	runtime := tui.New()
	runtime.Render(sh, tui.Size{W: 60, H: 10})

	// This cell is inside both the menu row and the later-drawn toast.
	if !runtime.Handle(input.MouseEvent{X: 22, Y: 7, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("toast-covered pointer was not consumed")
	}
	if popupHits != 0 || underlyingHits != 0 || sh.popup == nil {
		t.Fatalf("toast click-through popup/underlying/popup = %d/%d/%v", popupHits, underlyingHits, sh.popup)
	}

	sh.toasts = nil
	runtime.Render(sh, tui.Size{W: 60, H: 10})
	if !runtime.Handle(input.MouseEvent{X: 22, Y: 7, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("popup pointer was not consumed")
	}
	if popupHits != 1 || underlyingHits != 0 {
		t.Fatalf("popup routing hits = popup %d underlying %d", popupHits, underlyingHits)
	}

	menu = widget.NewMenu([]widget.MenuItem{{Label: "Open"}})
	menu.SetAnchor(20, 6)
	menu.OnDismiss(sh.closePopup)
	sh.popup = menu
	runtime.Render(sh, tui.Size{W: 60, H: 10})
	if !runtime.Handle(input.MouseEvent{X: 1, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) || underlyingHits != 0 {
		t.Fatal("outside-popup dismissal clicked through to retained button")
	}
}

func TestToastExpandsAndDismisses(t *testing.T) {
	sh := &Shell{
		cfg:    config.Default(),
		mv:     &MainView{Root: widget.NewText("main")},
		toasts: []*Toast{NewToast("Gateway error", "line one line two line three", Styles{})},
	}

	if sh.Toast().Expanded() {
		t.Fatal("toast starts expanded")
	}
	if !sh.Handle(input.KeyEvent{Key: input.KeyEnter}) || !sh.Toast().Expanded() {
		t.Fatal("Enter did not expand toast")
	}
	if !sh.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'x'}) || sh.Toast() != nil {
		t.Fatal("x did not dismiss toast")
	}
}

func TestToastDoesNotConsumeUnderlyingInput(t *testing.T) {
	cfg := config.Default()
	sh := &Shell{
		cfg:    cfg,
		mv:     &MainView{Root: widget.NewText("main")},
		toasts: []*Toast{NewToast("Gateway error", "boom", Styles{})},
	}

	if sh.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'z'}) {
		t.Fatal("passive toast consumed ordinary input")
	}
	if sh.overlay != nil {
		t.Fatal("shortcut leaked through toast and opened overlay")
	}

	if !sh.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'x'}) || sh.Toast() != nil {
		t.Fatal("x did not dismiss toast")
	}
}

func TestToastRendersOverShell(t *testing.T) {
	sh := &Shell{
		cfg: config.Default(),
		mv:  &MainView{Root: widget.NewText("main")},
	}
	sh.ShowToast("Message failed", assertErr("send failed"))

	buf := tui.New().Render(sh, tui.Size{W: 60, H: 8})
	if !bufferContains(buf, "Message failed") {
		t.Fatal("rendered shell does not contain toast title")
	}
	if !bufferContains(buf, "send failed") {
		t.Fatal("rendered shell does not contain toast detail")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func bufferContains(buf *screen.Buffer, want string) bool {
	for y := 0; y < buf.Height(); y++ {
		if rowText(buf, y) == want {
			return true
		}
		if containsText(rowText(buf, y), want) {
			return true
		}
	}
	return false
}

func containsText(s, want string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
