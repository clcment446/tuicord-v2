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
- **Global shortcuts while typing:** the focused widget gets keys first
  (`tui.App.handleFocused` → focused.Handle, only bubbling to root Shell on
  false). `widget.TextInput` used to insert *any* rune, so `ctrl+v` typed a
  literal "v" (and `ctrl+e`/`ctrl+k` were swallowed) instead of reaching the
  Shell's global switch. Fixed: TextInput declines Ctrl/Super-modified runes
  (returns false) so they bubble; Alt is left alone for AltGr-composed chars.
- **Staging:** `MainView.StageTempImage(path, filename, size)` queues a
  `queuedAttachment{temp: true}`. The pasted bytes are written to an
  `os.CreateTemp` file; `clearAttachments` deletes temp files (on send and on
  cancel). On Linux the send path opens the FD before clear unlinks it, so the
  in-flight upload still reads it.
- **Wayland/native paste bind:** a bracketed `input.PasteEvent` with empty text
  (what terminals emit when the clipboard holds an image and the user hits
  ctrl+shift+v — no text target) triggers a quiet image-paste attempt in
  `Shell.Handle`. Real image-less empty pastes are no-ops.
- **Preview:** `Shell.openAttachmentPreview` decodes staged image attachments
  (`media.Decode` + `media.Downscale`) and shows them in an overlay via
  `widget.NewKittyImageFrom` — the same Kitty path (with cell fallback) as inline
  chat media (chatview.go:456). Opens automatically after a paste and via
  `;preview`.
- **Live E2E:** `TestReadClipboardImageRoundTrip` (term, gated by
  `TUICORD_CLIP_E2E=1`) wl-copies a PNG and reads it back byte-identical.

Branch: feat/paste-images (off devstyly).
