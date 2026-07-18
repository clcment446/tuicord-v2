package term

import (
	"os/exec"
	"strings"
)

// imagePreference is the order image formats are preferred when the clipboard
// advertises several. PNG first (lossless, universally accepted by Discord).
var imagePreference = []struct {
	mime string
	ext  string
}{
	{"image/png", "png"},
	{"image/jpeg", "jpg"},
	{"image/jpg", "jpg"},
	{"image/gif", "gif"},
	{"image/webp", "webp"},
	{"image/bmp", "bmp"},
}

// ErrNoClipboardImage reports that the clipboard holds no image (it may hold
// text or be empty). Callers should treat it as a benign "nothing to paste".
var ErrNoClipboardImage = clipboardError("no image in the clipboard")

// ErrNoClipboardImageTool reports that no supported clipboard reader is
// installed.
var ErrNoClipboardImageTool = clipboardError("no clipboard image reader available (install wl-paste, xclip, or pngpaste)")

// ReadClipboardImage returns the image currently on the system clipboard along
// with a file extension for it (without a dot). It shells out to the first
// available reader — wl-paste (Wayland), xclip (X11), or pngpaste (macOS) —
// mirroring how CopyToClipboard picks a writer. It returns ErrNoClipboardImage
// when the clipboard holds no image, and ErrNoClipboardImageTool when no reader
// is installed.
func ReadClipboardImage() ([]byte, string, error) {
	switch {
	case toolAvailable("wl-paste"):
		return readImageWithList("wl-paste",
			[]string{"--list-types"},
			func(mime string) []string { return []string{"--type", mime, "--no-newline"} })
	case toolAvailable("xclip"):
		return readImageWithList("xclip",
			[]string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
			func(mime string) []string { return []string{"-selection", "clipboard", "-t", mime, "-o"} })
	case toolAvailable("pngpaste"):
		out, err := exec.Command("pngpaste", "-").Output()
		if err != nil {
			return nil, "", ErrNoClipboardImage
		}
		if len(out) == 0 {
			return nil, "", ErrNoClipboardImage
		}
		return out, "png", nil
	default:
		return nil, "", ErrNoClipboardImageTool
	}
}

// readImageWithList lists the clipboard's advertised types with listArgs, picks
// the most preferred image type, then reads it with readArgs(mime).
func readImageWithList(tool string, listArgs []string, readArgs func(mime string) []string) ([]byte, string, error) {
	listed, err := exec.Command(tool, listArgs...).Output()
	if err != nil {
		return nil, "", ErrNoClipboardImage
	}
	mime, ext, ok := pickImageMime(string(listed))
	if !ok {
		return nil, "", ErrNoClipboardImage
	}
	out, err := exec.Command(tool, readArgs(mime)...).Output()
	if err != nil {
		return nil, "", ErrNoClipboardImage
	}
	if len(out) == 0 {
		return nil, "", ErrNoClipboardImage
	}
	return out, ext, nil
}

// pickImageMime chooses the most preferred image MIME type present in a
// newline-separated list of clipboard types. It is pure so it can be tested
// without a clipboard.
func pickImageMime(list string) (mime, ext string, ok bool) {
	have := make(map[string]bool)
	for _, line := range strings.Split(list, "\n") {
		have[strings.ToLower(strings.TrimSpace(line))] = true
	}
	for _, pref := range imagePreference {
		if have[pref.mime] {
			return pref.mime, pref.ext, true
		}
	}
	return "", "", false
}

// toolAvailable reports whether an executable is on PATH, using the injectable
// lookPath so tests can simulate an environment.
func toolAvailable(name string) bool {
	_, err := lookPath(name)
	return err == nil
}
