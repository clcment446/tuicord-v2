---
name: vim-modal-focus-and-floating-profile
summary: Modal composer entry needs a post-routing root focus request, and transient floating popups must capture their own mouse drag events because they are not in the retained hit tree.
tags: [#tui, #vim, #focus, #composer, #popup, #drag, #profile]
impact: high
commit: 368b257 (dirty)
date: 2026-07-17
created_at: 2026-07-17T14:54:54+02:00
scope: internal/tui/tui/app.go, internal/ui/shell.go, internal/ui/profile_popup.go
---

## Problem

Vim normal mode needed `I` to transfer exact focus into the composer and `;q`
to return to chat. The user profile also had to remain a floating overlay while
supporting drag, but Shell popups are drawn outside the retained widget tree.

## Cause

Focused children receive keys before the root, and the runtime previously had
no safe way for a root-owned mode transition to request a specific focus owner.
Likewise, the generic drag manager only hit-tests retained widgets, so it cannot
discover a transient Shell popup.

## Resolution

`tui.FocusRequester` is polled after normal key routing and applies the exact
focus transfer (`internal/tui/tui/app.go:274-280`). Shell detects `;q`, removes
only that token, leaves composer contents intact, and requests ChatView focus
(`internal/ui/shell.go:510-523`). `ProfilePopup` captures all popup input and
implements title-bar drag from root mouse coordinates
(`internal/ui/profile_popup.go:184-240`), preventing click-through.

## Notes

Keep Vim traversal guarded by an explicit per-widget enable flag. Do not place
modal focus changes inside child widgets or expect overlay-only popups to appear
in the retained drag hit index.

Normal mode must also remove the composer from the focus ring, not merely make
it non-preferred: `TextInput.CanFocus` is gated by `focusEnabled`, and MainView
toggles it at construction and on mode changes (`internal/tui/widget/textinput.go:113-127`,
`internal/ui/main.go:139-141,1134-1143`). Name that setter
`SetInputFocusEnabled`; naming it `SetFocusEnabled` accidentally satisfies
`tui.FocusConfigurable`, causing the runtime's split-focus policy to disable
every text input.
