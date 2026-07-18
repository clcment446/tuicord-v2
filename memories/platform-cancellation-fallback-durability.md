---
name: platform-cancellation-fallback-durability
summary: Platform integrations must separate blocking protocol reads from lifecycle locks, preserve one shared context budget across tool fallbacks, and sync directories after atomic renames.
tags: [#captcha, #clipboard, #atomicfile, #config, #ci, #race]
impact: high
date: 2026-07-18
created_at: 2026-07-18T21:59:49+01:00
scope: internal/captcha/bidi.go, internal/tui/term/clipboard_image.go, internal/atomicfile, internal/config/config.go, .github/workflows/ci.yml
---

## Lessons

A WebSocket command must not hold the same mutex needed by `Close` while blocked in `ReadJSON`. A dedicated response reader can dispatch by command ID while callers select on their own context; a context-aware command gate preserves ordering, and closing the connection independently wakes all pending calls.

Clipboard readers should try every installed backend in preference order when a backend reports no usable image or command failure. Cancellation, deadline, and output-limit errors are terminal: do not restart work through another tool, and reuse the caller's context so the total operation remains bounded.

Durable atomic replacement requires syncing the temporary file before rename and syncing the parent directory after rename on Unix systems that support directory fsync. First-run config and color templates should go through the same atomic helper while preserving existing files.

The nested Arikawa module is a separate Go module, so both ordinary and race tests must be explicit CI steps from `third_party/arikawa`.
