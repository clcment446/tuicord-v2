package term

import "testing"

func TestCapabilitiesFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Capabilities
	}{
		{
			name: "truecolor colorterm",
			env:  map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"},
			want: Capabilities{TrueColor: true, Color256: true},
		},
		{
			name: "no color disables truecolor only",
			env:  map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "NO_COLOR": "1"},
			want: Capabilities{TrueColor: false, Color256: true},
		},
		{
			name: "kitty implies modern protocols",
			env:  map[string]string{"TERM": "xterm-kitty"},
			want: Capabilities{Color256: false, KittyKeyboard: true, SyncOutput: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capabilitiesFromEnv(tt.env)
			if got != tt.want {
				t.Fatalf("capabilitiesFromEnv() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
