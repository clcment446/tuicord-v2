package term

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

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

var ErrNoClipboardImage = clipboardError("no image in the clipboard")
var ErrNoClipboardImageTool = clipboardError("no clipboard image reader available (install wl-paste, xclip, or pngpaste)")
var ErrClipboardImageTooLarge = clipboardError("clipboard image exceeds the configured size limit")

const (
	defaultClipboardImageMaxBytes int64 = 25 << 20
	defaultClipboardImageTimeout        = 5 * time.Second
	clipboardTypeListMaxBytes     int64 = 64 << 10
)

// ReadClipboardImage retains the compatibility API while applying bounded
// defaults. UI callers should use ReadClipboardImageContext so Shell shutdown
// can cancel external clipboard tools.
func ReadClipboardImage() ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClipboardImageTimeout)
	defer cancel()
	return ReadClipboardImageContext(ctx, defaultClipboardImageMaxBytes)
}

// ReadClipboardImageContext reads an image with cancellation and an exact output
// cap. Both MIME discovery and extraction use CommandContext and max+1 reads.
func ReadClipboardImageContext(ctx context.Context, maxBytes int64) ([]byte, string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultClipboardImageMaxBytes
	}
	switch {
	case toolAvailable("wl-paste"):
		return readImageWithListContext(ctx, maxBytes, "wl-paste",
			[]string{"--list-types"},
			func(mime string) []string { return []string{"--type", mime, "--no-newline"} })
	case toolAvailable("xclip"):
		return readImageWithListContext(ctx, maxBytes, "xclip",
			[]string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
			func(mime string) []string { return []string{"-selection", "clipboard", "-t", mime, "-o"} })
	case toolAvailable("pngpaste"):
		out, err := commandOutput(ctx, maxBytes, "pngpaste", "-")
		if err != nil {
			return nil, "", clipboardCommandError(err)
		}
		if len(out) == 0 {
			return nil, "", ErrNoClipboardImage
		}
		return out, "png", nil
	default:
		return nil, "", ErrNoClipboardImageTool
	}
}

// readImageWithList is retained for package tests and compatibility.
func readImageWithList(tool string, listArgs []string, readArgs func(string) []string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClipboardImageTimeout)
	defer cancel()
	return readImageWithListContext(ctx, defaultClipboardImageMaxBytes, tool, listArgs, readArgs)
}

func readImageWithListContext(ctx context.Context, maxBytes int64, tool string, listArgs []string, readArgs func(string) []string) ([]byte, string, error) {
	listed, err := commandOutput(ctx, clipboardTypeListMaxBytes, tool, listArgs...)
	if err != nil {
		return nil, "", clipboardCommandError(err)
	}
	mime, ext, ok := pickImageMime(string(listed))
	if !ok {
		return nil, "", ErrNoClipboardImage
	}
	out, err := commandOutput(ctx, maxBytes, tool, readArgs(mime)...)
	if err != nil {
		return nil, "", clipboardCommandError(err)
	}
	if len(out) == 0 {
		return nil, "", ErrNoClipboardImage
	}
	return out, ext, nil
}

func commandOutput(ctx context.Context, maxBytes int64, name string, args ...string) ([]byte, error) {
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes+1))
	if int64(len(out)) > maxBytes {
		cancel()
		_ = cmd.Wait()
		return nil, ErrClipboardImageTooLarge
	}
	if readErr != nil {
		cancel()
		_ = cmd.Wait()
		return nil, readErr
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return out, nil
}

func clipboardCommandError(err error) error {
	switch {
	case errors.Is(err, ErrClipboardImageTooLarge):
		return ErrClipboardImageTooLarge
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("clipboard image extraction: %w", err)
	default:
		return ErrNoClipboardImage
	}
}

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

func toolAvailable(name string) bool {
	_, err := lookPath(name)
	return err == nil
}
