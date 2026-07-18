package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func TestShellInputModeIAndComposerSemicolonQ(t *testing.T) {
	mv := &MainView{
		cfg:            config.Config{Accessibility: config.Accessibility{VimNavigation: true}},
		composer:       widget.NewTextInput("Message"),
		composerStatus: widget.NewText(""),
		chat:           NewChatView(nil, nil, nil, Styles{}),
	}
	mv.composer.SetInputFocusEnabled(false)
	s := &Shell{mv: mv, cfg: config.Config{Accessibility: config.Accessibility{VimNavigation: true}}}
	if !s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i did not enter input mode")
	}
	if !s.inputMode || !mv.composer.CanFocus() || s.TakeFocusRequest() != mv.composer {
		t.Fatal("input mode did not request composer focus")
	}
	mv.composer.SetValue("hello;q")
	s.composerChanged(mv.composer.Value(), mv.composer.Cursor())
	if s.inputMode || mv.composer.CanFocus() || mv.composer.Value() != "hello" {
		t.Fatalf(";q exit = mode %v value %q, want normal mode and hello", s.inputMode, mv.composer.Value())
	}
	if s.TakeFocusRequest() != mv.chat {
		t.Fatal(";q did not request chat focus")
	}
}

func TestVimITransfersRuntimeFocusToNewlyEnabledComposer(t *testing.T) {
	cfg := config.Config{Accessibility: config.Accessibility{VimNavigation: true}}
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
	if !shell.inputMode || !composer.CanFocus() {
		t.Fatal("i did not leave the composer in Vim input mode")
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
	mv := &MainView{
		cfg:            config.Config{Accessibility: config.Accessibility{VimNavigation: true}},
		composer:       widget.NewTextInput("Message"),
		composerStatus: widget.NewText(""),
		chat:           NewChatView(nil, nil, nil, Styles{}),
	}
	s := &Shell{mv: mv, cfg: config.Config{Accessibility: config.Accessibility{VimNavigation: true}}}
	mv.composer.SetInputFocusEnabled(false)
	mv.composer.OnBackspaceEmpty(s.leaveInputMode)
	if !s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'i'}) {
		t.Fatal("i did not enter input mode")
	}
	if !mv.composer.Handle(input.KeyEvent{Key: input.KeyBackspace}) {
		t.Fatal("empty composer backspace was not consumed")
	}
	if s.inputMode || mv.composer.CanFocus() || s.TakeFocusRequest() != mv.chat {
		t.Fatalf("empty backspace exit = mode %v canFocus %v request %T", s.inputMode, mv.composer.CanFocus(), s.TakeFocusRequest())
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
	if mv.composerMode != composerReply || !s.inputMode || !mv.composer.CanFocus() || s.TakeFocusRequest() != mv.composer {
		t.Fatalf("reply did not enter composer input mode: mode=%v input=%v", mv.composerMode, s.inputMode)
	}
}
