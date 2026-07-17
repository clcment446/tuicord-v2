---
name: modified-enter-requires-keyboard-protocol
summary: Shift+Enter only reaches the multiline composer when the terminal session enables an enhanced keyboard protocol; widget-only KeyEvent tests do not verify the terminal boundary.
tags: [#composer, #keyboard, #kitty, #terminal, #shift-enter, #tui]
impact: high
commit: 368b257 (dirty)
date: 2026-07-17
created_at: 2026-07-17T14:31:06+02:00
scope: internal/tui/term/term.go, internal/tui/input/parser.go, internal/tui/widget/textinput.go
---

## Problem

`TextInput.Handle` correctly inserted a newline for a synthetic
`KeyEnter|Shift` event, but physical Shift+Enter still submitted the message.

## Cause

Legacy terminal input encodes Enter and Shift+Enter as the same carriage-return
byte. The parser already decoded Kitty `CSI 13;2u` with modifiers
(`internal/tui/input/parser.go:241-264`), but terminal initialization never
enabled the Kitty keyboard protocol, so that sequence was never emitted.

## Resolution

Kitty-capable sessions now push disambiguated keyboard reporting during startup
and pop it before terminal restoration (`internal/tui/term/term.go:35-38,
96-106,160-174,260-274`). The composer consumes the resulting modified Enter
at `internal/tui/widget/textinput.go:407-412`. Parser, terminal-session, and
parser-to-widget regression tests pass, as does `go test ./...`.

## Notes

Do not treat a directly constructed modified `KeyEvent` as end-to-end coverage
for a terminal keybinding. Verify the terminal capability sequence, raw parser
input, and widget behavior together. Terminals without an enhanced keyboard
protocol cannot distinguish Shift+Enter from Enter.
