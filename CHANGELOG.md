# Changelog

## 2026-07-21 — `fix: pin fold toggles and reserve media heights` (#28)

- Fold/unfold toggles (markdown headers and v2 list controls) pin the toggled
  line to the screen row it was activated at, so repeated cycles cannot creep
  the viewport.
- Embed images, thumbnails, and videos carry their source dimensions into the
  store, letting loading placeholders reserve the final height; video posters
  and GIF stills reserve their trailer row too, so media loads no longer
  expand messages and shift the view.

## 2026-07-21 — `fix: replies, forwards, viewport anchoring, profile card`

- Show reply references above reply content: who was replied to, a one-line
  preview of the original, and a deletion notice when the original is gone (#26).
- Render forwarded messages: the snapshot's content, attachments, stickers, and
  embeds appear behind a "↱ Forwarded" quote bar instead of an empty message (#27).
- Anchor a scrolled chat viewport to the message at its top, so folds/unfolds
  of embed v2 lists, async media loads, and edits above the viewport no longer
  shift the reading position (#28, #29). Configurable via the new
  `display.sticky_anchor` option, on by default (#30).
- Restore the floating profile card for `U`: role chips render in their role
  colors, the profile picture loads inline, and shared servers are listed (#18).

## 2026-07-16 — `feat: improve terminal composer input`

- Enable Kitty keyboard protocol support so modified Enter is distinguishable.
- Add wrapped multi-line text input, vertical cursor movement, and paste hooks.

## 2026-07-16 — `feat: expand keyboard-first chat workflows`

- Add opt-in Vim navigation, modal composer focus, and keyboard-aware menus.
- Add draggable user profile cards and stable shared-DM lookup.
- Support workspace-scoped file attachments through paste and `$path` imports.

## 2026-07-16 — `feat: enrich focused chat interactions`

- Add message-anchor selection with `V` and formatted multi-message copy with `Y`.
- Keep focused highlights inside embed content, including inline-media placeholder cells.
- Expose `messages.focused.fg` and `messages.focused.bg` color overrides.
- Preserve rich interaction metadata through embed and component framing.
