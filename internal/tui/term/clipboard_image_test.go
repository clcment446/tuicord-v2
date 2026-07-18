package term

import (
	"os/exec"
	"testing"
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

func TestReadClipboardImageNoTool(t *testing.T) {
	// With nothing on PATH, ReadClipboardImage reports that no reader exists.
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = orig }()

	if _, _, err := ReadClipboardImage(); err != ErrNoClipboardImageTool {
		t.Fatalf("err = %v, want ErrNoClipboardImageTool", err)
	}
}
