---
name: composer-reactions-and-embed-markup
summary: Reaction selection now reuses composer colon autocomplete, while embed titles must pass through markup rendering for emoji and header-level colors.
tags: [#reactions, #composer, #picker, #embeds, #markup, #emoji, #colors]
impact: normal
commit: ab0672e (dirty)
date: 2026-07-17
created_at: 2026-07-17T00:00:00+02:00
scope: internal/ui/shell.go, internal/ui/inline_picker.go, internal/ui/embedview.go, internal/markup/parser.go
---

## Problem

The add-reaction action opened a separate fullscreen emoji/sticker/GIF picker,
and embed titles were rendered as plain text. As a result, title custom emoji,
`-#` small text, and `#`-level colors from `colors.conf` did not render through
the normal markup/media/style pipeline.

## Cause

`Shell.openReactionPicker` and the Ctrl+E path constructed the retired `Picker`
widget. `renderEmbed` passed `Embed.Title` to `embedPlainLines`, bypassing
`markup.Parse`, custom-emoji media handling, and `messages.header{n}` styles.
Headers also treated their entire body as one plain span.

## Resolution

The fullscreen picker implementation and shortcut were removed. Reactions now
seed a `:` token in the composer and use `InlinePicker`, converting custom
emoji selections to Discord's `name:id` reaction form. Markup gained
`Kind_Small` for `-#` lines. Header parsing preserves the legacy plain-heading
span shape but emits inline spans with `HeaderLevel` when markup is present;
embed titles now render through `ChatView.renderContent`.

## Notes

`messages.small`, `embeds.title`, and `messages.header{n}` remain semantic
selectors, so existing `colors.conf` overrides apply without a separate embed
title parser. Full `go test ./...` and `git diff --check` pass.
