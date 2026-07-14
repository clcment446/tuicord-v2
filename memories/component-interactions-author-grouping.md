---
name: component-interactions-author-grouping
summary: Preserve message flags in component interaction payloads and collapse consecutive author headers in chat rendering.
tags: [#components, #interactions, #discord, #chat, #rendering]
impact: high
commit: f9fae04 (dirty)
date: 2026-07-14
created_at: 2026-07-14T00:00:00+02:00
scope: internal/app/app.go, internal/ui/chatview.go
---

## Problem

Interactive message controls were rendered and locally activated, but the
outgoing interaction payload discarded the source message's flags. Consecutive
messages from one author also repeated the author header for every message.

## Cause

`componentInteraction` did not model Discord's `message_flags` field, so
Components V2 context was lost when posting a user-originated interaction.
`ChatView.render` unconditionally appended an author line for every message.

## Resolution

`SubmitComponent` now copies `store.Message.Flags` into `message_flags`, with a
regression test for the Components V2 flag (`internal/app/app.go:118-130,817-830`,
`internal/app/component_submit_test.go:45-80`). Chat rendering compares
consecutive authors by ID (or name when IDs are unavailable) and renders one
header per run, while preserving changed pending/failed status headers
(`internal/ui/chatview.go:430-480`, `internal/ui/chatview_test.go:36-55`).

## Notes

Focused app and UI tests pass. The full suite's Discord integration tests still
require network access and fail in the sandbox during DNS lookup.
