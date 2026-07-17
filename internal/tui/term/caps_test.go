package term

import (
	"os"
	"path/filepath"
	"testing"

	"awesomeProject/internal/tui/screen"
)

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

func TestLoadKittyANSI16Palette(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current-theme.conf")
	contents := "color0 #010203\ncolor1 #aabbcc\nforeground #ffffff\ncolor15 #fefefe\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	palette, ok := loadKittyANSI16Palette(path)
	if !ok {
		t.Fatal("loadKittyANSI16Palette reported no palette")
	}
	if palette[0] != screen.RGB(1, 2, 3) || palette[1] != screen.RGB(0xaa, 0xbb, 0xcc) || palette[15] != screen.RGB(0xfe, 0xfe, 0xfe) {
		t.Fatalf("palette = %+v, want parsed Kitty ANSI colors", palette)
	}
}
