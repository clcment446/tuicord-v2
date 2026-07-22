---
name: remote-merge-skip-worktree-overrides
summary: Skip-worktree files can hide local overrides from stash and block a clean fast-forward; clear the flag, preserve the overrides in a named stash, then accept incompatible incoming implementations deliberately.
tags: [#git, #merge, #skip-worktree, #stash, #picker]
impact: high
commit: 11afe37 (dirty)
date: 2026-07-17
created_at: 2026-07-17T16:10:00+02:00
scope: internal/ui/inline_picker.go, internal/ui/shell.go
---

## Problem

Fast-forwarding `refactor/codebase-cleanup` to merged remote commit `11afe37`
was initially blocked by local `skip-worktree` edits that `git stash` did not
capture, notably `internal/ui/inline_picker.go` and its tests.

## Cause

The index marked those paths with `S`, so ordinary status/stash handling did
not expose their working-tree differences. The incoming branch added
`internal/ui/picker.go`, duplicating picker symbols from the local override.

## Resolution

Clear each skip-worktree flag with `git update-index --no-skip-worktree`, save
the local overrides in a named stash, fast-forward to the remote merge, and
restore the incoming picker when the two implementations duplicate symbols.
The displaced local picker override remains recoverable in a named stash.

## Notes

When resolving a conflict produced by `git stash pop`, Git labels the current
merged branch as `ours` and the stashed content as `theirs`; inspect the marker
labels before accepting a side.
