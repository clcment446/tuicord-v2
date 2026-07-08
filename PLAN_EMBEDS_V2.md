# Embeds V2 / Components V2 Plan

## Summary

Support Discord Components V2 with a feedback-first workflow: build a mocked
`examples/embed-v2` rich-message playground first, then use the same normalized
model and renderer for live Discord messages.

Components V2 messages use the component tree as the primary rich layout. The
client must keep existing V1 embed behavior while adding hierarchical support
for text displays, sections, thumbnails, media galleries, files, separators,
containers, action rows, buttons, selects, and unknown component fallbacks.

## Key Changes

- Add a `ComponentTree` field to `store.Message` while preserving the existing
  flattened `Components` field for legacy action rows.
- Persist message flags so `IS_COMPONENTS_V2` messages can be identified.
- Convert arikawa's typed V2 components into store-native component nodes.
- Render V2 component trees inside `ChatView` with terminal-first ergonomics:
  visible numeric shortcuts, mouse hit targets, disabled/pending/error states,
  stable scroll behavior, and readable fallbacks for unsupported pieces.
- Add `examples/embed-v2` with mocked messages covering legacy embeds,
  containers, sections, galleries, files, separators, buttons, selects, and
  unsupported component types.

## Test Plan

- Unit-test V2 conversion for the major component kinds and unknown fallback.
- Unit-test wide and narrow rendering of containers, sections, galleries,
  controls, disabled states, and fallback chips.
- Smoke-test the mock app with `TUI_EXAMPLE_RENDER=1 go run ./examples/embed-v2`.
- Run `env GOCACHE=/tmp/awesomeProject-go-build go test ./...`.

## Assumptions

- Broad Components V2 coverage is prioritized over Heavenly Dao-specific custom
  shortcuts, but game-bot ergonomics drive the control layout.
- Live interaction submission can be layered on after the mocked example and
  rendering pass; this phase records activation intent and renders feedback
  states without changing Discord server state.
- Existing V1 embeds and legacy flattened components remain compatible.
