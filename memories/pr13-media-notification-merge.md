---
name: pr13-media-notification-merge
summary: PR #13 had to preserve PR #14's adaptive GIF/video pipeline while selectively adding avatars, role gradients, and actionable notification stacks.
tags: [#merge, #media, #gif, #video, #notifications, #identity]
impact: high
commit: 4151936
date: 2026-07-18
created_at: 2026-07-18T19:00:00Z
scope: internal/ui/chatview.go, internal/ui/richblocks.go, internal/ui/shell.go, internal/store/store.go, internal/tui/tui/app.go
---

## Problem

PR #13 conflicted with PR #14 in chat media rendering. A side-only resolution
would either discard PR #14's playable video/viewer and adaptive animation
cadence or discard PR #13's avatars, role gradients, posters, and notification
navigation. The PR's sparse history identity hydration also overwrote guild
nicknames/avatars, and stacked toasts only routed mouse input to the newest one.

## Resolution

The merge retains PR #14's wall-clock GIF animator, visible-only `Animating`
contract, 500 ms idle/50 ms active ticks, downscaling, viewer hooks, and direct
video playback. It adds the PR #13 frame-retention cap with a copied slice,
Discord JPEG proxy posters wired to the direct video playback URL, author
avatars and role gradients, and notification stacking. Sparse identities now
only fill absent guild name/avatar fields while always refreshing username.
Errors remain persistent; informational and incoming-message toasts expire.
Mouse activation removes and activates the exact toast clicked.

## Verification

`go test ./... -count=1`, `go vet ./...`, and `git diff --check` passed before
merge. `go test -race ./...` later exposed asynchronous app-test/store access
races that remain part of the post-merge whole-codebase review.
