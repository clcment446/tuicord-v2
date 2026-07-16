---
name: composer-inline-autocomplete
summary: Composer autocomplete owns its typed token while open and replaces it through grapheme-safe byte ranges.
tags: [#composer, #picker, #emoji, #sticker, #mentions, #tui]
impact: normal
commit: d718420 (dirty)
date: 2026-07-15
created_at: 2026-07-15T16:13:52+02:00
scope: internal/ui/inline_picker.go, internal/ui/shell.go, internal/tui/widget/textinput.go
---

## Problem

The existing full-screen picker required an explicit shortcut and could not
complete composer tokens for emoji, stickers, channels, members, or roles.

## Cause

`TextInput` exposed insertion but no grapheme-safe range replacement, and the
shell had no composer-change hook to identify the token at the cursor.

## Resolution

`InlinePicker` (`internal/ui/inline_picker.go:19-230`) fuzzy-filters the
current guild catalog for `:`, `%`, `#`, `@`, and `&`. Shell observes composer
changes (`internal/ui/shell.go:359-429`), keeps the raw token synchronized
while the menu is open, and replaces it with native Discord mention syntax or
the configured fake-Nitro URL. `TextInput.Replace`
(`internal/tui/widget/textinput.go:379-432`) expands arbitrary offsets to full
grapheme boundaries before editing.

## Notes

Native sticker entries invoke `SendSticker`; unavailable custom emoji and
stickers reuse `buildCustomEntries` and `buildStickerEntries`, which already
encode the Nitro/fake-Nitro policy. `env GOCACHE=/tmp/awesomeProject-go-build
go test ./...` passed.

`Backspace` on an empty inline query must delete the trigger from the composer
before closing (`internal/ui/inline_picker.go`, `internal/ui/shell.go`);
otherwise the next trigger becomes part of the stale token (for example `&@`).
Custom emoji rows use the same cached asynchronous media fetch and stable Kitty
image/placement IDs as chat content. Favorites persist in `uistate.State` and
are toggled with `Alt+F` in the full picker.
