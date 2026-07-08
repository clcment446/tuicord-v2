package term

import (
	"encoding/base64"
	"io"
	"os/exec"
	"strings"
)

// OSC52 builds the OSC 52 escape sequence that asks the terminal to copy text
// to the system clipboard. The payload is base64-encoded per the spec and
// terminated with BEL (\a). Writing the result to the terminal works across SSH
// and inside multiplexers that pass OSC 52 through (tmux with set-clipboard on).
//
// It is pure and does no IO, so it can be unit-tested and reused wherever a
// terminal writer is available.
func OSC52(text string) []byte {
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	seq := "\x1b]52;c;" + enc + "\a"
	return []byte(seq)
}

// lookPath is indirected so tests can exercise tool selection without depending
// on what happens to be installed.
var lookPath = exec.LookPath

// CopyToClipboard copies text to the system clipboard. It first emits an OSC 52
// sequence to w (typically the terminal), which is the most portable path. When
// w is nil or the write fails, it falls back to a local clipboard tool
// (wl-copy, xclip, xsel, or pbcopy). It returns an error only when every path
// fails.
func CopyToClipboard(w io.Writer, text string) error {
	if w != nil {
		if _, err := w.Write(OSC52(text)); err == nil {
			return nil
		}
	}
	return copyViaTool(text)
}

// CopyToClipboard copies text using this terminal as the OSC 52 sink, with the
// external-tool fallback.
func (t *Terminal) CopyToClipboard(text string) error {
	if t == nil {
		return CopyToClipboard(nil, text)
	}
	return CopyToClipboard(t, text)
}

// clipboardTool names an external clipboard writer and the arguments that make
// it read the value from stdin.
type clipboardTool struct {
	name string
	args []string
}

// clipboardTools is the fallback preference order: Wayland, then X11, then
// macOS. The first one found on PATH wins.
var clipboardTools = []clipboardTool{
	{name: "wl-copy"},
	{name: "xclip", args: []string{"-selection", "clipboard"}},
	{name: "xsel", args: []string{"--clipboard", "--input"}},
	{name: "pbcopy"},
}

// selectClipboardTool returns the first available tool from clipboardTools
// using find to test availability. It is pure with respect to find, so tests
// can inject a fake PATH lookup.
func selectClipboardTool(find func(string) (string, error)) (clipboardTool, bool) {
	for _, tool := range clipboardTools {
		if _, err := find(tool.name); err == nil {
			return tool, true
		}
	}
	return clipboardTool{}, false
}

func copyViaTool(text string) error {
	tool, ok := selectClipboardTool(lookPath)
	if !ok {
		return errNoClipboardTool
	}
	cmd := exec.Command(tool.name, tool.args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// errNoClipboardTool is returned when OSC 52 is unavailable and no local
// clipboard tool is installed.
var errNoClipboardTool = clipboardError("no clipboard writer available (OSC 52 sink missing and no wl-copy/xclip/xsel/pbcopy on PATH)")

type clipboardError string

func (e clipboardError) Error() string { return string(e) }
