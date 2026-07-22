---
name: autobot-viewport-insert-focus
summary: Non-modal plugin viewport refreshes can briefly rebuild focus onto chat and cancel pending Vim INSERT entry unless that replacement is tolerated.
tags: [#autobot, #plugins, #viewport, #vim, #focus]
impact: high
commit: facd583 (dirty)
date: 2026-07-22
created_at: 2026-07-22T08:34:32Z
scope: internal/ui/shell.go:306-327, internal/ui/plugin_viewport_test.go:21-34
---

## Problem

With the autobot plugin viewport visible, entering Vim INSERT mode could fail
during a transient focus-ring rebuild. The pending composer focus request was
created, but the rebuild selected ChatView first and the editor left pending
mode before the request could be applied.

## Cause

`Shell.beginComposerInput` normally rejects a `FocusChangeReplace` while a
composer focus request is pending. Plugin viewports are non-modal and can be
refreshed independently of the retained focus tree, so this replacement is an
expected intermediate state for that popup.

## Resolution

Plugin viewport entry now permits one replacement while the composer request
is pending, and `Shell.Handle` continues global routing when a non-modal plugin
viewport declines an event. The regression test covers entering INSERT with a
visible viewport and an empty focus ring.

## Notes

Focused tests pass with `GOCACHE=/tmp/tuicord-go-build go test ./internal/ui
-run 'TestPluginViewportDoesNotBlockVimInsertEntry|TestPluginViewportPreservesVimInsertFocus'`.
