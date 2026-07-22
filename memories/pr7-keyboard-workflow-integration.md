---
name: pr7-keyboard-workflow-integration
summary: The keyboard-workflow branch needs the full picker retained and its Vim Shell focus implementation restored.
tags: [#merge, #keyboard, #vim, #picker, #composer, #github]
impact: high
commit: dirty
date: 2026-07-17
created_at: 2026-07-17T00:00:00+01:00
scope: internal/ui/shell.go, internal/ui/picker.go, internal/config/config.go, internal/app/app.go
---

## Problem

PR #7 conflicted with the already merged resize/DM/search and blank-message
fixes. After resolving the textual conflicts, it still did not compile: the
branch deleted `internal/ui/picker.go` while retaining its callers, added a Vim
input-mode test without the Shell implementation, and removed the picker key
from configuration.

## Cause

The PR head was an inconsistent snapshot. GitHub's conflict status described
only textual mergeability and there were no CI checks to expose the missing
source files and implementations.

## Resolution

The resolved branch keeps the picker implementation and tests from master,
restores the `ctrl+e` configuration/help entry, and implements Shell's
post-routing focus requests for Vim `I` and `;q`. File uploads accept empty
text when files are present, while whitespace-only
text without files remains rejected. Both rich DM recipients and recipient IDs
are retained because they serve separate mention and profile lookup paths.

## Notes

`GOCACHE=/tmp/tuicord-pr7-resolve-cache go test ./... -count=1` passes on the
resolved branch based on master after PRs #6 and #8.
