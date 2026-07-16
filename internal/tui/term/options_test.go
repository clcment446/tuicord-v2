package term

import "testing"

func TestTerminalOptionsTrackMouseMode(t *testing.T) {
	if got := terminalMouseSequences(Options{Mouse: true}); got.enable == "" || got.disable == "" {
		t.Fatalf("mouse-on sequences = %+v, want enable and disable sequences", got)
	}
	if got := terminalMouseSequences(Options{Mouse: false}); got.enable != "" || got.disable != "" {
		t.Fatalf("mouse-off sequences = %+v, want no mouse sequences", got)
	}
}
