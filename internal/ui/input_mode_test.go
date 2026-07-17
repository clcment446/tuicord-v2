package ui

import (
	"testing"

	"awesomeProject/internal/config"
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
	if !s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'I', Mods: input.Shift}) {
		t.Fatal("I did not enter input mode")
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
