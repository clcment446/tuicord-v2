---
name: focused-strike-and-role-grouped-members
summary: Focus-fill padding must remove inline text decoration attributes, and the member sidebar groups each guild member under their highest hoisted role.
tags: [#chat, #focus, #strikethrough, #members, #roles, #sidebar]
impact: normal
commit: 5768408 (dirty)
date: 2026-07-27
created_at: 2026-07-27T16:00:00+02:00
scope: internal/ui/chatview_transcript.go, internal/ui/main.go
---

## Problem

A fully struck focused chat line drew a strike across the remaining component
width. The members sidebar only alphabetized all guild members and did not use
cached role data.

## Cause

`drawFocusedChatLine` used the first segment style for trailing focus padding,
including `screen.Strike`. `refreshMembers` read map-backed members and sorted
only by name, ignoring `Member.RoleIDs` and cached `Role.Hoist`/`Position`.

## Resolution

At `internal/ui/chatview_transcript.go:793-808`, clear textual attributes from
the padding base while retaining its semantic colors and focus styling. At
`internal/ui/main.go:1400-1459`, group members under their highest-position
hoisted role, order groups by store role order, and alphabetize each group;
ungrouped users remain alphabetized. Regression tests are at
`internal/ui/chatview_test.go:1315-1334` and
`internal/ui/ordering_ui_test.go:162-187`.

## Notes

Only hoisted roles define member-list sections; color-only or stale role IDs are
ignored. Direct-message recipient rendering remains unchanged.
