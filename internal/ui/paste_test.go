package ui

import (
	"os"
	"path/filepath"
	"testing"

	"awesomeProject/internal/tui/widget"
)

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
