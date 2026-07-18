---
name: clipboard-image-paste
summary: Pasting images works by reading the clipboard ourselves via wl-paste/xclip/pngpaste, not via bracketed paste (which is text-only); staged as a temp-file attachment.
tags: [#clipboard, #images, #upload, #composer, #term]
impact: medium
commit: b8d228a (dirty)
date: 2026-07-18
created_at: 2026-07-18T00:00:00+02:00
scope: internal/tui/term/clipboard_image.go, internal/ui/shell.go, internal/ui/main.go, internal/config/config.go
---

## Why this shape

Terminals only deliver **text** through bracketed paste (`input.PasteEvent`), so
binary clipboard images never arrive that way. The client instead reads the
image itself with an external tool — `term.ReadClipboardImage()` tries wl-paste
(Wayland) → xclip (X11) → pngpaste (macOS), mirroring the writer preference in
clipboard.go. It lists advertised MIME types and picks png>jpeg>gif>webp>bmp
(`pickImageMime`, pure/tested).

## Key points

- **Trigger:** `Keys.PasteImage` (default `ctrl+v`) and the `;paste`/`;img`
  command, both calling `Shell.pasteImage`. `ctrl+v` is safe as a default
  because terminals' own text paste is `ctrl+shift+v`, which still flows through
  bracketed paste → the composer.
- **Staging:** `MainView.StageTempImage(path, filename, size)` queues a
  `queuedAttachment{temp: true}`. The pasted bytes are written to an
  `os.CreateTemp` file; `clearAttachments` deletes temp files (on send and on
  cancel). On Linux the send path opens the FD before clear unlinks it, so the
  in-flight upload still reads it.
- **Live E2E:** `TestReadClipboardImageRoundTrip` (term, gated by
  `TUICORD_CLIP_E2E=1`) wl-copies a PNG and reads it back byte-identical.

Branch: feat/paste-images (off devstyly).
