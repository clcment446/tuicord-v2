---
name: components-v2-top-level-text-display
summary: Discord Components V2 permits Text Display directly in a message component list; Arikawa must treat it as TopLevelComponent or an entire history page fails with "expected container". Identical permanent error toasts should coalesce with a repeat count.
tags: [#discord, #components-v2, #json, #history, #toast, #ux]
impact: high
commit: 1742a28 (dirty)
date: 2026-07-19
created_at: 2026-07-19T01:27:01+03:00
scope: third_party/arikawa/discord/component.go, internal/ui/toast.go, internal/ui/shell.go
---

## Problem

Opening a channel whose history contained a Components V2 message with a
top-level Text Display failed the complete REST page with `JSON decoding
failed: expected container, got *discord.TextDisplayComponent`. Retried loads
also stacked identical permanent `Discord error` toasts over the composer.

## Cause

The vendored Arikawa component union could parse component type 10, but
`TextDisplayComponent` did not implement its private `TopLevelComponent`
marker. Discord's component reference explicitly allows Text Display directly
in a message's `components` list.

## Resolution

`TextDisplayComponent` now implements the top-level marker, and a regression
test decodes a realistic message-history array containing a top-level Text
Display. `Shell.ShowToast` coalesces identical permanent errors, brings a
repeated older error to the front, and renders its occurrence count in the
title while preserving distinct errors in the stack.

## Verification

The full vendored Arikawa `go test ./...` suite and the root `go test ./...`
suite pass. The Arikawa suite needs network access for its existing live
gateway checks and a writable module cache for its voice dependency.
