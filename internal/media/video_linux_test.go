//go:build linux

package media

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoArgsUseFullPixelAreaWithoutClearingGraphics(t *testing.T) {
	p := NewVideoPlayer(Config{CellPixelWidth: 10, CellPixelHeight: 20})
	got := strings.Join(p.args("clip.mp4", Rect{X: 1, Y: 1, Cols: 78, Rows: 22}, "/tmp/player.sock"), " ")
	for _, want := range []string{
		"--vo-kitty-config-clear=no",
		"--vo-kitty-width=780",
		"--vo-kitty-height=440",
		"--vo-kitty-left=2",
		"--vo-kitty-top=2",
		"--keep-open=yes",
		"--input-ipc-server=/tmp/player.sock",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args missing %q: %s", want, got)
		}
	}
}

func TestStabilizeKittySHMCopiesAndRewritesFrame(t *testing.T) {
	dir := t.TempDir()
	sourceName := "mpv-kitty-source"
	wantPixels := []byte{1, 2, 3, 4, 5}
	if err := os.WriteFile(filepath.Join(dir, sourceName), wantPixels, 0o600); err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte(sourceName))
	packet := []byte("\x1b_Ga=T,t=s,f=24,m=1;" + payload + "\x1b\\")

	got, stablePath, err := stabilizeKittySHM(packet, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(stablePath)
	if stablePath == "" || stablePath == filepath.Join(dir, sourceName) {
		t.Fatalf("stable path = %q", stablePath)
	}
	stablePixels, err := os.ReadFile(stablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stablePixels) != string(wantPixels) {
		t.Fatalf("stable pixels = %v, want %v", stablePixels, wantPixels)
	}
	encodedName := base64.StdEncoding.EncodeToString([]byte(filepath.Base(stablePath)))
	if !strings.Contains(string(got), ";"+encodedName+"\x1b\\") || strings.Contains(string(got), payload) {
		t.Fatalf("rewritten packet = %q", got)
	}
}
