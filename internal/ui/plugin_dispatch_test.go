package ui

import (
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/widget"
)

type dispatchPluginHost struct {
	acceptKey bool
	keyCalls  int
}

func (*dispatchPluginHost) RunCommand(string, []string) bool { return false }
func (p *dispatchPluginHost) RunKey(string) bool {
	p.keyCalls++
	return p.acceptKey
}
func (*dispatchPluginHost) KeySpecs() []string     { return []string{"ctrl+j"} }
func (*dispatchPluginHost) CommandNames() []string { return nil }
func (*dispatchPluginHost) ApplyTheme(string) bool { return false }
func (*dispatchPluginHost) ThemeNames() []string   { return nil }

func TestShellPluginKeyHandledOnlyWhenDispatchAccepted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		accept bool
	}{
		{name: "accepted", accept: true},
		{name: "rejected", accept: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugins := &dispatchPluginHost{acceptKey: tc.accept}
			mv := &MainView{
				cfg:  config.Config{},
				chat: NewChatView(nil, nil, nil, Styles{}),
				Root: widget.NewText(""),
			}
			s := &Shell{mv: mv, cfg: config.Config{}, plugins: plugins}

			got := s.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j', Mods: input.Ctrl})
			if got != tc.accept {
				t.Fatalf("Handle = %v, want %v", got, tc.accept)
			}
			if plugins.keyCalls != 1 {
				t.Fatalf("RunKey calls = %d, want 1", plugins.keyCalls)
			}
		})
	}
}
