package ui

import (
	"testing"

	appcore "awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"

	"github.com/diamondburned/arikawa/v3/session"
)

func TestShellInputModeIAndComposerSemicolonQ(t *testing.T) {
	cfg := vimTestConfig()
	mv := &MainView{
		cfg:            cfg,
		composer:       widget.NewTextInput("Message"),
		composerStatus: widget.NewText(""),
		chat:           NewChatView(nil, nil, nil, Styles{}),
	}
	mv.composer.SetInputFocusEnabled(false)
	s := &Shell{mv: mv, cfg: cfg}
	if !s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i did not enter input mode")
	}
	request, ok := s.TakeFocusRequest()
	if s.editor.phase != editorFocusPending || !mv.composer.CanFocus() || !ok || request.Target != mv.composer {
		t.Fatal("input mode did not request composer focus")
	}
	mv.composer.SetValue("hello;q")
	s.composerChanged(mv.composer.Value(), mv.composer.Cursor())
	if s.editor.phase != editorNormal || mv.composer.CanFocus() || mv.composer.Value() != "hello" {
		t.Fatalf(";q exit = phase %v value %q, want normal mode and hello", s.editor.phase, mv.composer.Value())
	}
	request, ok = s.TakeFocusRequest()
	if !ok || request.Target != mv.chat {
		t.Fatal(";q did not request chat focus")
	}
}

func TestVimITransfersRuntimeFocusToNewlyEnabledComposer(t *testing.T) {
	cfg := vimTestConfig()
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "hello"})
	chat := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	chat.SetVimNavigation(true)
	composer := widget.NewTextInput("Message")
	composer.SetInputFocusEnabled(false)
	mv := &MainView{
		cfg:            cfg,
		composer:       composer,
		composerStatus: widget.NewText(""),
		chat:           chat,
	}
	mv.Root = widget.Column(chat, composer)
	shell := &Shell{mv: mv, cfg: cfg}
	runtime := tui.New()
	runtime.Render(shell, tui.Size{W: 40, H: 8})
	if runtime.Focus.Focused() != chat {
		t.Fatalf("initial focus = %T, want chat", runtime.Focus.Focused())
	}

	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i was not handled")
	}
	// The composer becomes focusable during handling and enters the ring on the
	// invalidated render that follows.
	runtime.Render(shell, tui.Size{W: 40, H: 8})
	if runtime.Focus.Focused() != composer {
		t.Fatalf("focus after i = %T, want composer", runtime.Focus.Focused())
	}
	if shell.editor.phase != editorInput || !composer.CanFocus() {
		t.Fatal("i did not leave the composer in Vim input mode")
	}
}

func TestVimShiftITransfersFocusToComposer(t *testing.T) {
	runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'I', Mods: input.Shift}) {
		t.Fatal("Shift+I was not handled")
	}
	runtime.Render(shell, tui.Size{W: 40, H: 9})
	if runtime.Focus.Focused() != mv.composer || shell.editor.phase != editorInput {
		t.Fatalf("Shift+I focus/phase = %T/%v, want composer/input", runtime.Focus.Focused(), shell.editor.phase)
	}
}

func TestShellEmptyVimInsertAndExitBindingsStayDisabled(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		cfg := vimTestConfig()
		cfg.Keys.Vim.Insert = ""
		runtime, shell, _, _ := newVimFocusHarness(t, cfg)
		if runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
			t.Fatal("disabled insert binding was handled")
		}
		if shell.editor.phase != editorNormal {
			t.Fatalf("disabled insert changed editor phase to %v", shell.editor.phase)
		}
	})

	t.Run("exit input", func(t *testing.T) {
		cfg := vimTestConfig()
		cfg.Keys.Vim.ExitInput = ""
		runtime, shell, _, _ := newVimFocusHarness(t, cfg)
		enterVimInput(t, runtime, shell)
		if runtime.Handle(input.KeyEvent{Key: input.KeyEsc}) {
			t.Fatal("disabled exit-input binding was handled")
		}
		if shell.editor.phase != editorInput {
			t.Fatalf("disabled exit-input changed editor phase to %v", shell.editor.phase)
		}
	})

	t.Run("all", func(t *testing.T) {
		cfg := vimTestConfig()
		cfg.Keys.Vim = config.VimKeys{}
		runtime, shell, _, _ := newVimFocusHarness(t, cfg)
		if runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
			t.Fatal("all-disabled Vim config still entered input mode")
		}
		if shell.editor.phase != editorNormal {
			t.Fatalf("all-disabled Vim config changed editor phase to %v", shell.editor.phase)
		}
	})
}

func TestNonVimExplicitFocusCancelsHiddenComposerRequest(t *testing.T) {
	cfg := config.Default()
	cfg.Accessibility.VimNavigation = false
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "hello"})
	chat := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	composer := widget.NewTextInput("Message")
	composer.SetPreferredFocus(false)
	composer.Layout().Hidden = true
	other := widget.NewButton("other", nil)
	mv := &MainView{cfg: cfg, composer: composer, composerStatus: widget.NewText(""), chat: chat}
	mv.Root = widget.Column(chat, composer, other)
	shell := &Shell{mv: mv, cfg: cfg}
	shell.bindEditorState()
	runtime := tui.New()
	runtime.Render(shell, tui.Size{W: 40, H: 9})

	if !runtime.Handle(input.KeyEvent{Key: input.KeyEsc}) {
		t.Fatal("configured composer-focus key was not handled")
	}
	if !runtime.Focus.Set(other) {
		t.Fatal("explicit focus move failed")
	}
	composer.Layout().Hidden = false
	runtime.Render(shell, tui.Size{W: 40, H: 9})
	if runtime.Focus.Focused() != other {
		t.Fatalf("stale non-Vim request stole focus: got %T, want other", runtime.Focus.Focused())
	}
}

func TestPopupEditFocusPreemptsChatAndTransfersToComposer(t *testing.T) {
	cfg := config.Config{Accessibility: config.Accessibility{VimNavigation: true}}
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "me", Content: "old"})
	chat := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	chat.SetVimNavigation(true)
	composer := widget.NewTextInput("Message")
	composer.SetInputFocusEnabled(false)
	mv := &MainView{cfg: cfg, composer: composer, composerStatus: widget.NewText(""), chat: chat}
	mv.Root = widget.Column(chat, composer)
	shell := &Shell{mv: mv, cfg: cfg}
	shell.popup = widget.NewMenu([]widget.MenuItem{{Label: "Edit", OnSelect: func() {
		shell.closePopup()
		mv.BeginEdit(store.Message{ID: 1, ChannelID: 1, Author: "me", Content: "old"})
		shell.focusComposer()
	}}})
	runtime := tui.New()
	runtime.Render(shell, tui.Size{W: 40, H: 8})

	if !runtime.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("popup did not consume Enter")
	}
	runtime.Render(shell, tui.Size{W: 40, H: 8})
	if shell.popup != nil {
		t.Fatal("Edit popup did not close")
	}
	if runtime.Focus.Focused() != composer {
		t.Fatalf("focus after popup Edit = %T, want composer", runtime.Focus.Focused())
	}
	if mv.composerMode != composerEdit || composer.Value() != "old" {
		t.Fatalf("edit state = mode %v value %q", mv.composerMode, composer.Value())
	}
}

func TestShellBackspaceOnEmptyComposerLeavesInputMode(t *testing.T) {
	cfg := vimTestConfig()
	mv := &MainView{
		cfg:            cfg,
		composer:       widget.NewTextInput("Message"),
		composerStatus: widget.NewText(""),
		chat:           NewChatView(nil, nil, nil, Styles{}),
	}
	s := &Shell{mv: mv, cfg: cfg}
	mv.composer.SetInputFocusEnabled(false)
	mv.composer.OnBackspaceEmpty(s.leaveInputMode)
	if !s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i did not enter input mode")
	}
	if !mv.composer.Handle(input.KeyEvent{Key: input.KeyBackspace}) {
		t.Fatal("empty composer backspace was not consumed")
	}
	request, ok := s.TakeFocusRequest()
	if s.editor.phase != editorNormal || mv.composer.CanFocus() || !ok || request.Target != mv.chat {
		t.Fatalf("empty backspace exit = phase %v canFocus %v request %T", s.editor.phase, mv.composer.CanFocus(), request.Target)
	}
}

func TestVimInputFocusMovementExitsWithoutOverridingDestination(t *testing.T) {
	tests := []struct {
		name string
		move func(*tui.App, *Shell, *MainView, tui.Widget)
		want func(*MainView, tui.Widget) tui.Widget
	}{
		{
			name: "click-away",
			move: func(runtime *tui.App, _ *Shell, _ *MainView, _ tui.Widget) {
				runtime.Handle(input.MouseEvent{X: 1, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress})
			},
			want: func(mv *MainView, _ tui.Widget) tui.Widget { return mv.chat },
		},
		{
			name: "Tab",
			move: func(runtime *tui.App, _ *Shell, _ *MainView, _ tui.Widget) {
				runtime.Handle(input.KeyEvent{Key: input.KeyTab})
			},
			want: func(_ *MainView, other tui.Widget) tui.Widget { return other },
		},
		{
			name: "Shift-Tab",
			move: func(runtime *tui.App, _ *Shell, _ *MainView, _ tui.Widget) {
				runtime.Handle(input.KeyEvent{Key: input.KeyTab, Mods: input.Shift})
			},
			want: func(mv *MainView, _ tui.Widget) tui.Widget { return mv.chat },
		},
		{
			name: "Alt history",
			move: func(runtime *tui.App, _ *Shell, _ *MainView, _ tui.Widget) {
				runtime.Handle(input.KeyEvent{Key: input.KeyLeft, Mods: input.Alt})
			},
			want: func(mv *MainView, _ tui.Widget) tui.Widget { return mv.chat },
		},
		{
			name: "direct Set",
			move: func(runtime *tui.App, _ *Shell, _ *MainView, other tui.Widget) {
				if !runtime.Focus.Set(other) {
					t.Fatal("direct focus Set failed")
				}
			},
			want: func(_ *MainView, other tui.Widget) tui.Widget { return other },
		},
		{
			name: "render replacement",
			move: func(runtime *tui.App, shell *Shell, _ *MainView, _ tui.Widget) {
				// Bypass the overlay helper to verify the runtime focus notification
				// itself enforces the invariant on retained-tree replacement.
				shell.overlay = NewHelpOverlay(shell.cfg, shell.styles)
				runtime.Render(shell, tui.Size{W: 40, H: 9})
			},
			want: func(_ *MainView, _ tui.Widget) tui.Widget { return nil },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, shell, mv, other := newVimFocusHarness(t, vimTestConfig())
			enterVimInput(t, runtime, shell)
			tt.move(runtime, shell, mv, other)
			want := tt.want(mv, other)
			if runtime.Focus.Focused() != want {
				t.Fatalf("focus = %T, want chosen destination %T", runtime.Focus.Focused(), want)
			}
			if shell.editor.phase != editorNormal || mv.composer.CanFocus() {
				t.Fatalf("phase/canFocus = %v/%v, want normal/false", shell.editor.phase, mv.composer.CanFocus())
			}
		})
	}
}

func TestVimIRejectsReadOnlyAndCanceledPendingRequestsNeverFire(t *testing.T) {
	t.Run("read-only i", func(t *testing.T) {
		runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
		mv.composer.SetReadOnly(true)
		if runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
			t.Fatal("read-only i was claimed")
		}
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if shell.editor.phase != editorNormal || runtime.Focus.Focused() != mv.chat || mv.composer.CanFocus() {
			t.Fatal("read-only i entered or focused the composer")
		}
	})

	tests := []struct {
		name   string
		cancel func(*Shell, *MainView)
	}{
		{name: "read-only", cancel: func(_ *Shell, mv *MainView) { mv.composer.SetReadOnly(true) }},
		{name: "independent overlay", cancel: func(shell *Shell, _ *MainView) { shell.setIndependentOverlay(NewHelpOverlay(shell.cfg, shell.styles)) }},
		{name: "channel", cancel: func(shell *Shell, _ *MainView) { shell.channelChanged(1, 2) }},
	}
	for _, tt := range tests {
		t.Run("pending "+tt.name, func(t *testing.T) {
			runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
			if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
				t.Fatal("i was not handled")
			}
			tt.cancel(shell, mv)
			runtime.Render(shell, tui.Size{W: 40, H: 9})
			if runtime.Focus.Focused() == mv.composer || shell.editor.phase != editorNormal || mv.composer.CanFocus() {
				t.Fatalf("stale %s request fired: focus=%T phase=%v", tt.name, runtime.Focus.Focused(), shell.editor.phase)
			}
			// Remove the canceling condition and render again: invalidation is
			// permanent, not a request delayed until the overlay/read-only state ends.
			switch tt.name {
			case "read-only":
				mv.composer.SetReadOnly(false)
			case "independent overlay":
				shell.closeOverlay()
			}
			runtime.Render(shell, tui.Size{W: 40, H: 9})
			if runtime.Focus.Focused() == mv.composer || shell.editor.phase != editorNormal || mv.composer.CanFocus() {
				t.Fatalf("stale %s request fired later", tt.name)
			}
		})
	}

	t.Run("pending click on current navigation owner", func(t *testing.T) {
		runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
		runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'})
		// The old ring still focuses chat. A same-owner click is nevertheless an
		// explicit destination choice and must cancel the deferred composer Set.
		runtime.Handle(input.MouseEvent{X: 1, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress})
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if runtime.Focus.Focused() != mv.chat || shell.editor.phase != editorNormal || mv.composer.CanFocus() {
			t.Fatal("same-owner click allowed pending composer focus to fire")
		}
	})
}

func TestVimEscapePreservesDraftAttachmentsAndOperationBeforeCancel(t *testing.T) {
	runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
	enterVimInput(t, runtime, shell)
	target := store.Message{ID: 7, ChannelID: 1, Author: "alice"}
	mv.BeginReply(target, true)
	mv.composer.SetValue("draft text")
	mv.attachments = []queuedAttachment{{meta: store.Attachment{Filename: "draft.png"}}}

	if !runtime.Handle(input.KeyEvent{Key: input.KeyEsc}) {
		t.Fatal("first Esc was not handled")
	}
	if shell.editor.phase != editorNormal || runtime.Focus.Focused() != mv.chat {
		t.Fatalf("first Esc phase/focus = %v/%T", shell.editor.phase, runtime.Focus.Focused())
	}
	if mv.composer.Value() != "draft text" || mv.composerMode != composerReply || mv.composerTarget.ID != target.ID || len(mv.attachments) != 1 {
		t.Fatalf("first Esc destroyed composer state: value=%q mode=%v target=%v attachments=%d", mv.composer.Value(), mv.composerMode, mv.composerTarget.ID, len(mv.attachments))
	}

	if !runtime.Handle(input.KeyEvent{Key: input.KeyEsc}) {
		t.Fatal("second Esc did not cancel reply mode")
	}
	if mv.composerMode != composerNormal || mv.composerTarget.ID != 0 {
		t.Fatal("second normal-mode Esc did not cancel reply operation")
	}
}

func TestComposerOwnedPickerRestoresInputIndependentOverlayDoesNot(t *testing.T) {
	t.Run("inline picker", func(t *testing.T) {
		runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
		enterVimInput(t, runtime, shell)
		picker := NewInlinePicker(store.New(0), Styles{}, 0, 0, false, false, ':', "", func(string) {}, nil, shell.closeOverlay)
		shell.setComposerOverlay(picker)
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if shell.editor.phase != editorOverlaySuspended || runtime.Focus.Focused() != picker || mv.composer.CanFocus() {
			t.Fatal("composer picker did not suspend exact input ownership")
		}
		if !runtime.Handle(input.KeyEvent{Key: input.KeyEsc}) {
			t.Fatal("picker Esc was not handled")
		}
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if shell.editor.phase != editorInput || runtime.Focus.Focused() != mv.composer || !mv.composer.CanFocus() {
			t.Fatalf("picker close did not restore composer input: phase=%v focus=%T", shell.editor.phase, runtime.Focus.Focused())
		}
	})

	t.Run("independent overlay", func(t *testing.T) {
		runtime, shell, mv, _ := newVimFocusHarness(t, vimTestConfig())
		enterVimInput(t, runtime, shell)
		shell.setIndependentOverlay(NewHelpOverlay(shell.cfg, shell.styles))
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if shell.editor.phase != editorNormal || mv.composer.CanFocus() {
			t.Fatal("independent overlay retained input mode")
		}
		runtime.Handle(input.KeyEvent{Key: input.KeyEsc})
		runtime.Render(shell, tui.Size{W: 40, H: 9})
		if runtime.Focus.Focused() != mv.chat || shell.editor.phase != editorNormal || mv.composer.CanFocus() {
			t.Fatal("independent overlay silently restored composer input")
		}
	})
}

func TestConfiguredNextPanelAndFocusComposerBindingsClaimRuntimeKeys(t *testing.T) {
	cfg := vimTestConfig()
	cfg.Keys.NextPanel = "n"
	cfg.Keys.FocusComposer = "g"
	runtime, shell, mv, other := newVimFocusHarness(t, cfg)
	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'g'}) {
		t.Fatal("configured focus_composer key was not claimed")
	}
	runtime.Render(shell, tui.Size{W: 40, H: 9})
	if runtime.Focus.Focused() != mv.composer || shell.editor.phase != editorInput {
		t.Fatal("configured focus_composer did not enter exact input focus")
	}
	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'n'}) {
		t.Fatal("configured next_panel key was not claimed")
	}
	if runtime.Focus.Focused() != other || shell.editor.phase != editorNormal || mv.composer.CanFocus() {
		t.Fatalf("configured next_panel focus/phase = %T/%v", runtime.Focus.Focused(), shell.editor.phase)
	}
	buf := tui.New().Render(NewHelpOverlay(cfg, Styles{}), tui.Size{W: 60, H: 24})
	if !bufferContains(buf, "n") || !bufferContains(buf, "i / g") {
		t.Fatal("help does not expose configured panel/composer bindings")
	}
}

func TestUnfocusedComposerDoesNotReceiveShellFallbackKeyOrPaste(t *testing.T) {
	cfg := config.Config{}
	composer := widget.NewTextInput("Message")
	nav := widget.NewButton("navigation", nil)
	chat := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	mv := &MainView{cfg: cfg, composer: composer, composerStatus: widget.NewText(""), chat: chat}
	mv.Root = widget.Column(nav, composer)
	shell := &Shell{mv: mv, cfg: cfg}
	runtime := tui.New()
	runtime.Render(shell, tui.Size{W: 30, H: 3})
	if !runtime.Focus.Set(nav) {
		t.Fatal("could not focus navigation widget")
	}
	runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'x'})
	runtime.Handle(input.PasteEvent{Text: "secret paste"})
	if composer.Value() != "" {
		t.Fatalf("unfocused composer received hidden fallback input: %q", composer.Value())
	}
}

func TestChannelChangeClearsEditorRequestAndComposerTarget(t *testing.T) {
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 9, Kind: store.ChannelText})
	st.UpsertChannel(store.Channel{ID: 2, GuildID: 9, Kind: store.ChannelText})
	runtime := tui.New()
	logic := appcore.New(discord.WrapSession(session.New("")), st, runtime)
	logic.SetActive(9, 1)
	cfg := vimTestConfig()
	composer := widget.NewTextInput("Message")
	composer.SetInputFocusEnabled(false)
	mv := &MainView{app: logic, cfg: cfg, composer: composer, composerStatus: widget.NewText(""), chat: NewChatView(st, logic.ActiveChannel, nil, Styles{}), lastActiveChannel: 1}
	shell := &Shell{app: logic, mv: mv, cfg: cfg}
	shell.bindEditorState()
	mv.SetChannelChangeHandler(shell.channelChanged)
	mv.BeginEdit(store.Message{ID: 11, ChannelID: 1, Content: "old"})
	shell.focusComposer()

	mv.setActive(9, 2)
	if shell.editor.phase != editorNormal || shell.focusRequestActive || composer.CanFocus() {
		t.Fatal("channel change retained editor state or exact focus request")
	}
	if mv.composerMode != composerNormal || mv.composerTarget.ID != 0 {
		t.Fatal("channel change retained cross-channel edit target")
	}

	// Bypass the centralized path to exercise the submission guard itself.
	mv.composerMode = composerReply
	mv.composerTarget = store.Message{ID: 12, ChannelID: 1}
	mv.composer.SetValue("must not reply")
	mv.onSend(mv.composer.Value())
	if mv.composerMode != composerNormal || mv.composerTarget.ID != 0 {
		t.Fatal("defense-in-depth target check did not reject stale reply")
	}
}

func TestShellVimReplyEntersComposerInputMode(t *testing.T) {
	mv := &MainView{
		cfg:            config.Config{Accessibility: config.Accessibility{VimNavigation: true}},
		composer:       widget.NewTextInput("Message"),
		composerStatus: widget.NewText(""),
		chat:           NewChatView(nil, nil, nil, Styles{}),
	}
	mv.composer.SetInputFocusEnabled(false)
	s := &Shell{mv: mv, cfg: config.Config{Accessibility: config.Accessibility{VimNavigation: true}}}
	s.handleMessageAction('r', store.Message{ID: 1, ChannelID: 2, Author: "alice"})
	request, ok := s.TakeFocusRequest()
	if mv.composerMode != composerReply || s.editor.phase != editorFocusPending || !mv.composer.CanFocus() || !ok || request.Target != mv.composer {
		t.Fatalf("reply did not enter composer input mode: mode=%v phase=%v", mv.composerMode, s.editor.phase)
	}
}

func vimTestConfig() config.Config {
	cfg := config.Default()
	cfg.Accessibility.VimNavigation = true
	return cfg
}

func newVimFocusHarness(t *testing.T, cfg config.Config) (*tui.App, *Shell, *MainView, tui.Widget) {
	t.Helper()
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "hello"})
	chat := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	chat.SetVimNavigation(true)
	composer := widget.NewTextInput("Message")
	composer.SetPreferredFocus(false)
	composer.SetInputFocusEnabled(false)
	other := widget.NewButton("other", nil)
	mv := &MainView{cfg: cfg, composer: composer, composerStatus: widget.NewText(""), chat: chat}
	mv.Root = widget.Column(chat, composer, other)
	shell := &Shell{mv: mv, cfg: cfg}
	shell.bindEditorState()
	composer.OnBackspaceEmpty(shell.leaveInputMode)
	runtime := tui.New()
	runtime.Render(shell, tui.Size{W: 40, H: 9})
	if runtime.Focus.Focused() != chat {
		t.Fatalf("initial focus = %T, want chat", runtime.Focus.Focused())
	}
	return runtime, shell, mv, other
}

func enterVimInput(t *testing.T, runtime *tui.App, shell *Shell) {
	t.Helper()
	if !runtime.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i was not handled")
	}
	runtime.Render(shell, tui.Size{W: 40, H: 9})
	if shell.editor.phase != editorInput || runtime.Focus.Focused() != shell.mv.composer {
		t.Fatalf("input entry phase/focus = %v/%T", shell.editor.phase, runtime.Focus.Focused())
	}
}
