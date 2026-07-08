package term

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os/exec"
	"testing"
)

func TestOSC52Encoding(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"ascii", "1234567890"},
		{"empty", ""},
		{"unicode", "héllo 🎉"},
		{"newlines", "line1\nline2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OSC52(tt.in)
			prefix := []byte("\x1b]52;c;")
			if !bytes.HasPrefix(got, prefix) {
				t.Fatalf("missing OSC 52 prefix: %q", got)
			}
			if got[len(got)-1] != '\a' {
				t.Fatalf("missing BEL terminator: %q", got)
			}
			payload := got[len(prefix) : len(got)-1]
			decoded, err := base64.StdEncoding.DecodeString(string(payload))
			if err != nil {
				t.Fatalf("payload is not valid base64: %v", err)
			}
			if string(decoded) != tt.in {
				t.Fatalf("round-trip = %q, want %q", decoded, tt.in)
			}
		})
	}
}

func TestCopyToClipboardWritesOSC52(t *testing.T) {
	var buf bytes.Buffer
	if err := CopyToClipboard(&buf, "copy me"); err != nil {
		t.Fatalf("CopyToClipboard error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), OSC52("copy me")) {
		t.Fatalf("writer got %q, want OSC 52 sequence", buf.Bytes())
	}
}

// failWriter always errors, forcing the external-tool fallback.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestCopyToClipboardFallsBackWhenWriteFails(t *testing.T) {
	// No tool on the fake PATH → fallback fails with the sentinel error.
	restore := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = restore }()

	err := CopyToClipboard(failWriter{}, "x")
	if err == nil {
		t.Fatal("expected an error when OSC 52 fails and no tool exists")
	}
}

func TestSelectClipboardToolPrefersWayland(t *testing.T) {
	find := func(name string) (string, error) {
		if name == "wl-copy" || name == "xclip" {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	tool, ok := selectClipboardTool(find)
	if !ok || tool.name != "wl-copy" {
		t.Fatalf("selected %+v (ok=%v), want wl-copy", tool, ok)
	}
}

func TestSelectClipboardToolFallsThroughToX11(t *testing.T) {
	find := func(name string) (string, error) {
		if name == "xclip" {
			return "/usr/bin/xclip", nil
		}
		return "", exec.ErrNotFound
	}
	tool, ok := selectClipboardTool(find)
	if !ok || tool.name != "xclip" {
		t.Fatalf("selected %+v (ok=%v), want xclip", tool, ok)
	}
	if len(tool.args) == 0 {
		t.Fatal("xclip needs -selection clipboard args")
	}
}

func TestSelectClipboardToolNoneAvailable(t *testing.T) {
	find := func(string) (string, error) { return "", exec.ErrNotFound }
	if _, ok := selectClipboardTool(find); ok {
		t.Fatal("expected no tool when PATH is empty")
	}
}

func TestNilTerminalCopyUsesFallback(t *testing.T) {
	restore := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = restore }()

	var nilT *Terminal
	if err := nilT.CopyToClipboard("x"); err == nil {
		t.Fatal("nil terminal with no tool should return an error, not panic")
	}
}
