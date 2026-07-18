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
	return readClipboardImageContext(ctx, maxBytes, toolAvailable, commandOutput)
}

type clipboardImageOutput func(context.Context, int64, string, ...string) ([]byte, error)

func readClipboardImageContext(ctx context.Context, maxBytes int64, available func(string) bool, output clipboardImageOutput) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes <= 0 {
		maxBytes = defaultClipboardImageMaxBytes
	}
	installed := false
	lastErr := error(ErrNoClipboardImage)
	try := func(name string, read func() ([]byte, string, error)) ([]byte, string, bool, error) {
		if !available(name) {
			return nil, "", false, nil
		}
		installed = true
		data, ext, err := read()
		if err == nil {
			return data, ext, true, nil
		}
		lastErr = err
		if errors.Is(err, ErrClipboardImageTooLarge) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, "", true, err
		}
		return nil, "", false, nil
	}

	if data, ext, done, err := try("wl-paste", func() ([]byte, string, error) {
		return readImageWithListCommand(ctx, maxBytes, "wl-paste",
			[]string{"--list-types"},
			func(mime string) []string { return []string{"--type", mime, "--no-newline"} }, output)
	}); done {
		return data, ext, err
	}
	if data, ext, done, err := try("xclip", func() ([]byte, string, error) {
		return readImageWithListCommand(ctx, maxBytes, "xclip",
			[]string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
			func(mime string) []string { return []string{"-selection", "clipboard", "-t", mime, "-o"} }, output)
	}); done {
		return data, ext, err
	}
	if data, ext, done, err := try("pngpaste", func() ([]byte, string, error) {
		out, err := output(ctx, maxBytes, "pngpaste", "-")
		if err != nil {
			return nil, "", clipboardCommandError(err)
		}
		if len(out) == 0 {
			return nil, "", ErrNoClipboardImage
		}
		return out, "png", nil
	}); done {
		return data, ext, err
	}
	if !installed {
		return nil, "", ErrNoClipboardImageTool
	}
	return nil, "", lastErr
}

// readImageWithList is retained for package tests and compatibility.
func readImageWithList(tool string, listArgs []string, readArgs func(string) []string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClipboardImageTimeout)
	defer cancel()
	return readImageWithListContext(ctx, defaultClipboardImageMaxBytes, tool, listArgs, readArgs)
}

func readImageWithListContext(ctx context.Context, maxBytes int64, tool string, listArgs []string, readArgs func(string) []string) ([]byte, string, error) {
	return readImageWithListCommand(ctx, maxBytes, tool, listArgs, readArgs, commandOutput)
}

func readImageWithListCommand(ctx context.Context, maxBytes int64, tool string, listArgs []string, readArgs func(string) []string, output clipboardImageOutput) ([]byte, string, error) {
	listed, err := output(ctx, clipboardTypeListMaxBytes, tool, listArgs...)
	if err != nil {
		return nil, "", clipboardCommandError(err)
	}
	mime, ext, ok := pickImageMime(string(listed))
	if !ok {
		return nil, "", ErrNoClipboardImage
	}
	out, err := output(ctx, maxBytes, tool, readArgs(mime)...)
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
