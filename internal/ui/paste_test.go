package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIsImageFilename(t *testing.T) {
	for _, name := range []string{"a.png", "b.JPG", "c.jpeg", "d.gif", "e.webp", "f.bmp"} {
		if !isImageFilename(name) {
			t.Errorf("isImageFilename(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"a.txt", "b.pdf", "noext", "c.mp4"} {
		if isImageFilename(name) {
			t.Errorf("isImageFilename(%q) = true, want false", name)
		}
	}
}

func TestImageAttachmentsFilters(t *testing.T) {
	mv := &MainView{attachments: []queuedAttachment{
		{path: "/tmp/a.png", meta: store.Attachment{Filename: "a.png"}},
		{path: "/tmp/b.txt", meta: store.Attachment{Filename: "b.txt"}},
		{path: "/tmp/c.gif", meta: store.Attachment{Filename: "c.gif"}},
	}}
	got := mv.imageAttachments()
	if len(got) != 2 || got[0].meta.Filename != "a.png" || got[1].meta.Filename != "c.gif" {
		t.Fatalf("imageAttachments = %+v", got)
	}
}

func TestBuildImagePreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.png")
	writePNG(t, path, 400, 300)

	mv := &MainView{previewCellW: 8, previewCellH: 16, styles: Styles{}}
	p, ok := mv.buildImagePreview(path)
	if !ok {
		t.Fatal("buildImagePreview failed on a valid PNG")
	}
	// The thumbnail must fit the composer budget.
	if p.cols < 1 || p.cols > composerPreviewMaxCols {
		t.Fatalf("cols = %d, want 1..%d", p.cols, composerPreviewMaxCols)
	}
	if p.rows < 1 || p.rows > composerPreviewMaxRows {
		t.Fatalf("rows = %d, want 1..%d", p.rows, composerPreviewMaxRows)
	}
	// The measured size matches the cell footprint.
	if got := p.Measure(tui.Size{}); got.W != p.cols || got.H != p.rows {
		t.Fatalf("Measure = %+v, want %dx%d", got, p.cols, p.rows)
	}
	if _, ok := mv.buildImagePreview(filepath.Join(t.TempDir(), "missing.png")); ok {
		t.Fatal("buildImagePreview reported ok for a missing file")
	}
}

func TestUpdateComposerPreviewTracksAttachments(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	writePNG(t, img, 200, 120)

	mv := &MainView{
		composerFiles:   widget.NewText(""),
		composerPreview: widget.Column(),
		composerNode:    &layout.Node{Basis: composerBaseBasis},
		previewCellW:    8,
		previewCellH:    16,
		styles:          Styles{},
	}
	if err := mv.StageTempImage(img, "a.png", 100); err != nil {
		t.Fatal(err)
	}
	if mv.composerPreview.Layout().Hidden || mv.composerNode.Basis <= composerBaseBasis {
		t.Fatalf("preview not shown/grown: hidden=%v basis=%d", mv.composerPreview.Layout().Hidden, mv.composerNode.Basis)
	}
	mv.clearAttachments()
	if !mv.composerPreview.Layout().Hidden || mv.composerNode.Basis != composerBaseBasis {
		t.Fatalf("preview not reset after clear: hidden=%v basis=%d", mv.composerPreview.Layout().Hidden, mv.composerNode.Basis)
	}
}

func TestStageTempImageQueuesAndCleansUp(t *testing.T) {
	mv := &MainView{composerFiles: widget.NewText("")}
	path := filepath.Join(t.TempDir(), "clip.png")
	if err := os.WriteFile(path, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := mv.StageTempImage(path, "pasted.png", 7); err != nil {
		t.Fatalf("StageTempImage: %v", err)
	}
	if len(mv.attachments) != 1 || !mv.attachments[0].temp || mv.attachments[0].meta.Filename != "pasted.png" {
		t.Fatalf("attachment not staged as temp: %+v", mv.attachments)
	}

	// Staging the same path twice does not duplicate it.
	if err := mv.StageTempImage(path, "pasted.png", 7); err != nil {
		t.Fatalf("StageTempImage (dup): %v", err)
	}
	if len(mv.attachments) != 1 {
		t.Fatalf("duplicate temp image was queued: %+v", mv.attachments)
	}

	// Clearing removes the queue and deletes the temp file.
	mv.clearAttachments()
	if len(mv.attachments) != 0 {
		t.Fatalf("attachments not cleared: %+v", mv.attachments)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file not removed on clear (stat err = %v)", err)
	}
}

func TestStageTempImageRejectsOversizeAndReadOnly(t *testing.T) {
	mv := &MainView{composerFiles: widget.NewText("")}
	if err := mv.StageTempImage("/tmp/x.png", "x.png", MaxUploadBytes+1); err == nil {
		t.Fatal("oversized image was accepted")
	}
	if len(mv.attachments) != 0 {
		t.Fatal("oversized image was queued")
	}

	ro := &MainView{composerFiles: widget.NewText(""), composerReadOnly: true}
	if err := ro.StageTempImage("/tmp/x.png", "x.png", 10); err == nil {
		t.Fatal("read-only composer accepted an attachment")
	}
}
