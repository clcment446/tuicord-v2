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

func TestReadClipboardImageNoTool(t *testing.T) {
	// With nothing on PATH, ReadClipboardImage reports that no reader exists.
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = orig }()

	if _, _, err := ReadClipboardImage(); err != ErrNoClipboardImageTool {
		t.Fatalf("err = %v, want ErrNoClipboardImageTool", err)
	}
}
