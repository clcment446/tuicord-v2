package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
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
