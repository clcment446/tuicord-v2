//go:build linux

package media

import (
	"bytes"
	"encoding/base64"
	"errors"
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

func TestStabilizeKittySHMRejectsOversizeWithoutLeavingSnapshot(t *testing.T) {
	dir := t.TempDir()
	sourceName := "mpv-kitty-oversize"
	if err := os.WriteFile(filepath.Join(dir, sourceName), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte(sourceName))
	packet := []byte("\x1b_Ga=T,t=s,f=24,m=1;" + payload + "\x1b\\")

	if _, path, _, err := stabilizeKittySHMWithLimit(packet, dir, 4); !errors.Is(err, ErrVideoFrameTooLarge) {
		t.Fatalf("oversize error = %v, path = %q", err, path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != sourceName {
		t.Fatalf("oversize snapshot cleanup left entries %v", entries)
	}
}

func TestOversizeSHMFrameDropsWholeAPCAndPreservesFollowingOutput(t *testing.T) {
	dir := t.TempDir()
	sourceName := "mpv-kitty-oversize-stream"
	if err := os.WriteFile(filepath.Join(dir, sourceName), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte(sourceName))
	packet := []byte("\x1b_Ga=T,t=s,f=24,m=1;" + payload + "\x1b\\")
	following := []byte("\x1b[2J")
	input := append(append([]byte(nil), packet...), following...)

	var framer kittyOutputFramer
	var session videoSession
	var got []byte
	for _, chunk := range framer.Push(input) {
		if stable, ok := session.snapshotKittyChunk(chunk, dir, 4, 8); ok {
			got = append(got, stable...)
		}
	}
	if tail := framer.Flush(); len(tail) > 0 {
		if stable, ok := session.snapshotKittyChunk(tail, dir, 4, 8); ok {
			got = append(got, stable...)
		}
	}
	if !bytes.Equal(got, following) {
		t.Fatalf("output after oversized SHM frame = %q, want %q", got, following)
	}
}

func TestVideoSessionRetainedSHMUsesByteBudgetAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	if err := os.WriteFile(first, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("5678"), 0o600); err != nil {
		t.Fatal(err)
	}

	var session videoSession
	if !session.trackSHM(first, 4, 6) || !session.trackSHM(second, 4, 6) {
		t.Fatal("snapshots within the individual byte limit were rejected")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("old snapshot was not evicted: %v", err)
	}
	if session.shmBytes != 4 || len(session.shmFiles) != 1 || session.shmFiles[0].path != second {
		t.Fatalf("retained snapshots = %+v (%d bytes)", session.shmFiles, session.shmBytes)
	}

	session.cleanupSHM()
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("cleanup retained snapshot: %v", err)
	}
	if session.shmBytes != 0 || len(session.shmFiles) != 0 {
		t.Fatalf("cleanup accounting = %+v (%d bytes)", session.shmFiles, session.shmBytes)
	}
}
