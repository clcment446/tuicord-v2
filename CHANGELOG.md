# Changelog

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
