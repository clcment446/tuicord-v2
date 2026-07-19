---
name: vim-editor-interaction-invariant
summary: Vim composer mode must be a root-owned state machine driven by runtime focus notifications and generation-validated exact focus requests.
tags: [#vim, #composer, #focus, #overlay, #channel, #keyboard, #mouse, #tui]
impact: critical
date: 2026-07-19
scope: internal/tui/tui, internal/tui/widget, internal/ui/shell.go, internal/ui/main.go
---

## Problem

A Shell input-mode boolean, a separate MainView display boolean, and a deferred
widget pointer could disagree. Focus changes through click, Tab, history, direct
Set, or tree replacement left the hidden composer editable, while stale `i`
requests could fire after a read-only/channel/overlay transition. Root fallback
also broadcast ordinary keys and paste through container children, reaching an
unfocused input.

## Resolution

Shell now owns one editor interaction phase: normal, exact-composer-focus
pending, input, or composer-overlay suspended. `FocusManager` reports every
focus assignment to the root, including same-owner pointer/traversal choices
that cancel a request against an old ring. Exact requests carry an opaque
generation, remain valid across the single enabling render, and are revalidated
before every deferred attempt. Composer-owned pickers suspend and explicitly
restore; independent overlays and channel changes invalidate restoration.

Runtime fallback bubbles only along the focused ancestry. Forwarding layout
containers implement `HandleBubble` so an ancestor cannot rebroadcast into
siblings. The final focus ring comes from visible non-zero hit rectangles,
with a retained-tree fallback only for embedders that expose no layout hits.

Channel activation runs through MainView's centralized hook, clearing stale
reply/edit targets and editor requests. Submission independently rejects a
target whose channel differs from the active channel. Popup/toast pointer
routing follows reverse draw order, and modal widgets receive Tab before global
focus traversal.

## Regression coverage

Real runtime tests cover click/Tab/Shift-Tab/history/direct/replacement focus,
read-only `i`, canceled pending generations, Esc draft/attachment/operation
preservation, picker restore, independent overlay non-restore, configured focus
claims, channel target cleanup, hidden input routing, hidden focusables, and
popup/toast z-order.
