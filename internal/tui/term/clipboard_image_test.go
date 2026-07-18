package term

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestPickImageMime(t *testing.T) {
	cases := []struct {
		name     string
		list     string
		wantMime string
		wantExt  string
		wantOK   bool
	}{
		{
			name:     "prefers png over jpeg",
			list:     "text/plain\nimage/jpeg\nimage/png\n",
			wantMime: "image/png",
			wantExt:  "png",
			wantOK:   true,
		},
		{
			name:     "falls back to jpeg",
			list:     "TARGETS\nimage/jpeg\ntext/html",
			wantMime: "image/jpeg",
			wantExt:  "jpg",
			wantOK:   true,
		},
		{
			name:     "case and whitespace insensitive",
			list:     "  IMAGE/GIF \n",
			wantMime: "image/gif",
			wantExt:  "gif",
			wantOK:   true,
		},
		{
			name:   "no image type present",
			list:   "text/plain\ntext/html\nUTF8_STRING",
			wantOK: false,
		},
		{
			name:   "empty list",
			list:   "",
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mime, ext, ok := pickImageMime(tc.list)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (mime != tc.wantMime || ext != tc.wantExt) {
				t.Fatalf("got (%q, %q), want (%q, %q)", mime, ext, tc.wantMime, tc.wantExt)
			}
		})
	}
}

func TestClipboardCommandOutputCapsAtMaxPlusOne(t *testing.T) {
	_, err := commandOutput(context.Background(), 4, "sh", "-c", "printf 12345")
	if !errors.Is(err, ErrClipboardImageTooLarge) {
		t.Fatalf("commandOutput error = %v, want too large", err)
	}
}

func TestClipboardCommandOutputHonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := commandOutput(ctx, 1024, "sh", "-c", "while :; do :; done")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("commandOutput error = %v, want deadline exceeded", err)
	}
}

func TestReadClipboardImageFallsBackAfterInstalledToolFailure(t *testing.T) {
	available := func(string) bool { return true }
	var calls []string
	output := func(_ context.Context, _ int64, name string, args ...string) ([]byte, error) {
		calls = append(calls, name)
		switch name {
		case "wl-paste":
			return nil, errors.New("wayland clipboard unavailable")
		case "xclip":
			if hasArg(args, "TARGETS") {
				return []byte("image/png\n"), nil
			}
			return []byte("x11-image"), nil
		default:
			return nil, errors.New("unexpected tool")
		}
	}

	data, ext, err := readClipboardImageContext(context.Background(), 1024, available, output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "x11-image" || ext != "png" {
		t.Fatalf("image = (%q, %q), want xclip png", data, ext)
	}
	if len(calls) < 2 || calls[0] != "wl-paste" || calls[1] != "xclip" {
		t.Fatalf("tool calls = %v, want wl-paste then xclip", calls)
	}
}

func TestReadClipboardImageFallsBackToPngpaste(t *testing.T) {
	available := func(string) bool { return true }
	var calls []string
	output := func(_ context.Context, _ int64, name string, _ ...string) ([]byte, error) {
		calls = append(calls, name)
		if name == "pngpaste" {
			return []byte("mac-image"), nil
		}
		return nil, errors.New("clipboard backend failed")
	}

	data, ext, err := readClipboardImageContext(context.Background(), 1024, available, output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mac-image" || ext != "png" {
		t.Fatalf("image = (%q, %q), want pngpaste png", data, ext)
	}
	want := []string{"wl-paste", "xclip", "pngpaste"}
	if len(calls) != len(want) {
		t.Fatalf("tool calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("tool calls = %v, want %v", calls, want)
		}
	}
}

func TestReadClipboardImageDoesNotFallbackAfterBoundedFailure(t *testing.T) {
	for _, wantErr := range []error{context.DeadlineExceeded, ErrClipboardImageTooLarge} {
		t.Run(wantErr.Error(), func(t *testing.T) {
			var calls []string
			output := func(_ context.Context, _ int64, name string, _ ...string) ([]byte, error) {
				calls = append(calls, name)
				return nil, wantErr
			}
			_, _, err := readClipboardImageContext(context.Background(), 4,
				func(string) bool { return true }, output)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if len(calls) != 1 || calls[0] != "wl-paste" {
				t.Fatalf("tool calls = %v, want only wl-paste", calls)
			}
		})
	}
}

func TestReadClipboardImageNoTool(t *testing.T) {
	// With nothing on PATH, ReadClipboardImage reports that no reader exists.
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = orig }()

	if _, _, err := ReadClipboardImage(); err != ErrNoClipboardImageTool {
		t.Fatalf("err = %v, want ErrNoClipboardImageTool", err)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
