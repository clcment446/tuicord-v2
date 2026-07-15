package term

import (
	"strings"
	"testing"
)

func TestTerminalOptionsTrackMouseMode(t *testing.T) {
	if got := terminalMouseSequences(Options{Mouse: true}); got.enable == "" || got.disable == "" {
		t.Fatalf("mouse-on sequences = %+v, want enable and disable sequences", got)
	}
	if got := terminalMouseSequences(Options{Mouse: false}); got.enable != "" || got.disable != "" {
		t.Fatalf("mouse-off sequences = %+v, want no mouse sequences", got)
	}
}

func TestTerminalSessionEnablesKittyKeyboardWhenSupported(t *testing.T) {
	seq := terminalSessionSequences(Options{Mouse: true}, Capabilities{KittyKeyboard: true})
	if !strings.Contains(seq.enable, pushKittyKeyboard) {
		t.Fatalf("enable sequence %q does not push Kitty keyboard mode", seq.enable)
	}
	if !strings.Contains(seq.disable, popKittyKeyboard) {
		t.Fatalf("disable sequence %q does not pop Kitty keyboard mode", seq.disable)
	}
	if strings.Index(seq.disable, popKittyKeyboard) > strings.Index(seq.disable, leaveAltScreen) {
		t.Fatalf("Kitty mode is restored after leaving alternate screen: %q", seq.disable)
	}
}

func TestTerminalSessionSkipsKittyKeyboardWhenUnsupported(t *testing.T) {
	seq := terminalSessionSequences(Options{}, Capabilities{})
	if strings.Contains(seq.enable, pushKittyKeyboard) || strings.Contains(seq.disable, popKittyKeyboard) {
		t.Fatalf("unsupported terminal received Kitty keyboard sequences: %+v", seq)
	}
}
