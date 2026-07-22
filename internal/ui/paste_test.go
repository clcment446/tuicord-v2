package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/app"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"

	"github.com/diamondburned/arikawa/v3/session"
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

	style := Styles{}.Cell("messages.content")
	p, ok := decodeImagePreview(path, 8, 16, style)
	if !ok {
		t.Fatal("decodeImagePreview failed on a valid PNG")
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
	if _, ok := decodeImagePreview(filepath.Join(t.TempDir(), "missing.png"), 8, 16, style); ok {
		t.Fatal("decodeImagePreview reported ok for a missing file")
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
		previewCellW:      8,
		previewCellH:      16,
		styles:            Styles{},
		syncPreviewDecode: true,
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

// TestUpdateComposerPreviewDecodesOffUIThread proves the default path defers the
// decode: after staging, the thumbnail is not yet cached (it is decoding on a
// goroutine) and the path is marked pending.
func TestUpdateComposerPreviewDecodesOffUIThread(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	writePNG(t, img, 200, 120)

	mv := &MainView{
		app:             app.New(discord.WrapSession(session.New("")), store.New(0), tui.New()),
		composerFiles:   widget.NewText(""),
		composerPreview: widget.Column(),
		composerNode:    &layout.Node{Basis: composerBaseBasis},
		previewCellW:    8,
		previewCellH:    16,
		styles:          Styles{},
		// syncPreviewDecode left false: production behavior.
	}
	if err := mv.StageTempImage(img, "a.png", 100); err != nil {
		t.Fatal(err)
	}
	if _, cached := mv.previewCache[img]; cached {
		t.Fatal("thumbnail was decoded synchronously on the UI thread")
	}
	if !mv.previewPending[img] {
		t.Fatal("off-thread decode was not marked pending")
	}
}

// TestStaleDecodeDroppedAfterClearAndRestage proves the epoch guard: an
// off-thread decode that started before clearAttachments must not populate the
// cache after the same temp path is re-staged, since its bytes may be from the
// old file that once lived at that reused path. A fresh decode is issued instead.
func TestStaleDecodeDroppedAfterClearAndRestage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	writePNG(t, img, 200, 120)

	mv := &MainView{
		app:             app.New(discord.WrapSession(session.New("")), store.New(0), tui.New()),
		composerFiles:   widget.NewText(""),
		composerPreview: widget.Column(),
		composerNode:    &layout.Node{Basis: composerBaseBasis},
		previewCellW:    8,
		previewCellH:    16,
		styles:          Styles{},
	}
	// Stage P: an async decode is now in flight at the current epoch.
	if err := mv.StageTempImage(img, "a.png", 100); err != nil {
		t.Fatal(err)
	}
	staleEpoch := mv.previewEpoch
	if !mv.previewPending[img] {
		t.Fatal("staging did not launch an off-thread decode")
	}

	// Clear (bumps the epoch, drops the pending guard) then re-stage the SAME path.
	mv.clearAttachments()
	if mv.previewPending != nil && mv.previewPending[img] {
		t.Fatal("clearAttachments left a stale pending guard for the reused path")
	}
	if err := mv.StageTempImage(img, "a.png", 100); err != nil {
		t.Fatal(err)
	}
	if !mv.previewPending[img] {
		t.Fatal("re-stage did not issue a fresh decode for the reused temp path")
	}

	// The original in-flight decode posts its (stale) result back. It must be
	// dropped: caching it would show the OLD file's bytes for the reused path.
	stale := &imagePreview{cols: 1, rows: 1}
	mv.applyDecodedPreview(img, staleEpoch, stale, true)
	if got := mv.previewCache[img]; got == stale {
		t.Fatal("stale in-flight decode populated previewCache with the old image")
	}

	// The fresh decode (current epoch) still populates normally.
	fresh := &imagePreview{cols: 2, rows: 2}
	mv.applyDecodedPreview(img, mv.previewEpoch, fresh, true)
	if got := mv.previewCache[img]; got != fresh {
		t.Fatalf("fresh decode did not populate previewCache: got %v", got)
	}
}

// TestSyncPreviewDecodePopulatesImmediately confirms the test decode seam still
// caches inline with no event loop after the epoch-guard change.
func TestSyncPreviewDecodePopulatesImmediately(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	writePNG(t, img, 200, 120)

	mv := &MainView{
		composerFiles:     widget.NewText(""),
		composerPreview:   widget.Column(),
		composerNode:      &layout.Node{Basis: composerBaseBasis},
		previewCellW:      8,
		previewCellH:      16,
		styles:            Styles{},
		syncPreviewDecode: true,
	}
	if err := mv.StageTempImage(img, "a.png", 100); err != nil {
		t.Fatal(err)
	}
	if _, ok := mv.previewCache[img]; !ok {
		t.Fatal("sync decode did not populate previewCache immediately")
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

func TestClipboardPasteRemovesTempWhenTryPostRejects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rejected.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	lifecycle, closeLifecycle := context.WithCancel(context.Background())
	defer closeLifecycle()
	op, cancelOp := context.WithCancel(context.Background())
	sh := &Shell{
		lifecycleCtx:    lifecycle,
		mv:              &MainView{},
		tryPost:         func(func()) bool { return false },
		clipboardBusy:   true,
		clipboardCancel: cancelOp,
	}
	sh.finishClipboardPaste(op, cancelOp, false, []byte("png"), "png", path, nil)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected post stranded temp file: %v", err)
	}
	if sh.clipboardBusy || sh.clipboardCancel != nil {
		t.Fatal("rejected post left clipboard operation busy")
	}
}

func TestClipboardPasteDeadlineClearsBusyAndReportsTimeout(t *testing.T) {
	lifecycle, closeLifecycle := context.WithCancel(context.Background())
	defer closeLifecycle()
	op, cancelOp := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelOp()
	sh := &Shell{
		lifecycleCtx:    lifecycle,
		mv:              &MainView{},
		clipboardBusy:   true,
		clipboardCancel: cancelOp,
		tryPost: func(fn func()) bool {
			fn()
			return true
		},
	}

	sh.finishClipboardPaste(op, cancelOp, false, nil, "", "", context.DeadlineExceeded)

	if sh.clipboardBusy || sh.clipboardCancel != nil {
		t.Fatal("deadline left clipboard operation busy")
	}
	if len(sh.toasts) != 1 || sh.toasts[0].title != "Paste image" || !strings.Contains(sh.toasts[0].detail, "timed out") {
		t.Fatalf("deadline notice = %#v", sh.toasts)
	}
}

func TestClipboardPasteClosureRechecksShellShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accepted.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	lifecycle, closeLifecycle := context.WithCancel(context.Background())
	op, cancelOp := context.WithCancel(context.Background())
	var posted func()
	mv := &MainView{}
	sh := &Shell{
		lifecycleCtx: lifecycle,
		mv:           mv,
		tryPost: func(fn func()) bool {
			posted = fn
			return true
		},
	}
	sh.finishClipboardPaste(op, cancelOp, false, []byte("png"), "png", path, nil)
	if posted == nil {
		t.Fatal("clipboard completion was not posted")
	}
	closeLifecycle()
	posted()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("shutdown-raced closure stranded temp file: %v", err)
	}
	if len(mv.attachments) != 0 {
		t.Fatal("shutdown-raced closure staged an attachment")
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
