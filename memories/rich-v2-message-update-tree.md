---
name: rich-v2-message-update-tree
summary: Components V2 edits must replace the renderer-backed ComponentTree during MESSAGE_UPDATE handling.
tags: [#components-v2, #embeds, #renderer, #message-update]
impact: high
commit: f9fae04 (dirty)
date: 2026-07-14
created_at: 2026-07-14T00:00:00+02:00
scope: internal/app/app.go:601-620, internal/app/rich_test.go:88-120
---

## Problem

Editing a Discord message containing Components V2 content left the terminal renderer showing the previous rich block. The update test covered legacy embeds but not the V2 tree consumed by `internal/ui/componentview.go`.

## Cause

`handleMessageUpdate` copied `Components` but omitted `ComponentTree` (and the message `Flags`) from the converted gateway patch. The renderer prefers `Message.ComponentTree` whenever it is present, so it kept rendering the old tree.

## Resolution

The update patch now copies `Flags` and `ComponentTree` alongside the other rich-content fields in `internal/app/app.go:606-614`. `TestMessageUpdatePatchesComponentsV2TreeForRenderer` covers an edit from old to new V2 text in `internal/app/rich_test.go:88-120`.

## Notes

The focused regression and `go test ./...` both pass. When changing the message-update merge, test both legacy embed slices and the hierarchical V2 component tree because they use different renderer inputs.
