---
name: vim-gg-transcript-bounds
summary: Vim chat navigation uses gg to jump to the oldest loaded transcript boundary and G to return to the newest messages.
tags: [#vim, #chat, #navigation, #scroll, #history]
impact: normal
date: 2026-07-18
scope: internal/ui/chatview.go, internal/ui/chatview_test.go, internal/ui/help.go
---

## Behavior

In Vim navigation mode, the first unmodified `g` starts a pending sequence and
the second `g` jumps to and focuses the oldest currently loaded message.
Reaching that boundary invokes the existing older-history callback. While the
requested page is prepended, ChatView stays anchored to and moves focus onto the
new oldest message instead of drifting back toward the previous viewport.

Uppercase `G` (Shift+G) clears the bottom-relative offset, focuses the newest
message, and returns to the newest viewport. Any other motion or modified key
clears a pending lone `g`, preventing a later unrelated `g` from completing a
stale sequence. Shift remains accepted for uppercase Vim commands such as `G`,
`V`, and `Y`; Ctrl/Alt/Super runes do not enter Vim command handling.

## Bounded-history requirement

`G` cannot return to the live edge if prepending older pages evicts the newest
cached messages. At capacity, `Store.PrependMessagesSince` therefore retains a
split oldest/newest window: the oldest slots keep pagination moving, the newest
slots preserve the live conversation, and post-request mutations remain
protected. Repeated prepends may evict the middle of a very large transcript,
but they never discard the newest edge merely because the user scrolled up.
