---
name: graphics-layer-occlusion
summary: Overlays (popups/menus/toasts) occlude inline Kitty images via a per-cell draw-layer stamp on the screen buffer; partially covered images are re-clipped around the overlay (up to 4 sub-placements), not grayed out. z stays -1 for all graphics.
tags: [#kitty, #graphics, #overlay, #popup, #zindex, #occlusion, #screen, #media]
impact: high
commit: b137861 (dirty)
date: 2026-07-21
created_at: 2026-07-21T00:00:00+02:00
scope: internal/tui/screen/buffer.go, internal/tui/screen/diff.go, internal/tui/screen/types.go, internal/tui/widget/image.go, internal/tui/tui/app.go, internal/ui/profile_popup.go, internal/ui/picker.go, internal/ui/inline_picker.go, internal/tui/widget/itemlist.go
---

## Why

Inline images place at Kitty `z=-1`, which is **above cell backgrounds** and only
below text glyphs. So a popup/menu/toast drawn over an image showed its text but
the image **bled through the popup's blank/padding background cells**. No single z
fixes it: `z < -1073741824` hides images behind their own chat cells' backgrounds.
The correct fix is occlusion — delete the placement where a higher UI layer covers
it. (Whole-placement suppression was tried first and was wrong: it grayed out the
*entire* banner when a menu covered only its middle — the user rejected that.)

## How it works

- `screen.Buffer` has a draw-layer counter + `cellLayer []uint8` parallel to
  cells. `Set`/`clearCell` stamp `b.layer`; `Clear` resets to 0. `SetLayer(n)`
  bumps it. Each `Graphic` records the layer it was added on.
- `tui/app.go drawTree` draws the retained tree at layer 0, then calls
  `buf.SetLayer(1)` before the `DrawOverlay` loop, so popup/toast cells stamp
  layer 1. Occlusion is automatic and relies on overlays filling their footprint
  (menus/profile/toast do).
- `Buffer.resolveGraphics()` (used by `GraphicDiff` for **both** prev and next):
  a graphic covered by a strictly-higher layer is dropped (Clear emitted) if
  fully covered or has no `Reclip`; otherwise it is re-clipped. The occluder is
  the **bounding box** of higher-layer cells within the graphic rect;
  `subtractRect` yields up to 4 visible sub-rects (top/bottom full-width strips +
  left/right side strips).
- `Graphic.Reclip func([]Rect) []byte` + `ClearAll []byte` live on the graphic.
  The Image widget (`image.go`) sets `Reclip` to re-emit `kittyPlaceCropped` for
  each visible sub-rect (placement ids `base..base+maxReclipStrips-1`, 4). `ClearAll`
  deletes exactly those placement ids (NOT `d=i,i=<id>` which would wipe *every*
  placement of the image — fatal when the same avatar/URL appears twice, e.g. a
  DM author avatar and the profile popup sharing an image id). Resolved split
  graphics set `Clear=ClearAll` and
  `split=true`; `GraphicDiff` forces `Clear` when `old.split||next.split` and the
  Data changed (the same-payload fast path would otherwise leave stale
  sub-placements). Unchanged occlusion diffs to nothing (placements retained).
- **Seam alignment** (split rendering, LANDED): Kitty images upload at an exact
  cell-multiple pixel size — `snapToCellMultiple(fullPixel, cells)` (round to
  nearest multiple) feeds both `kittyUpload` and `kittyPlaceCropped`. Every cell
  then maps to an exact integer `unit = uploadPx/cells` block, so all re-clipped
  strips are pixel-perfect sub-windows of one shared grid: no seam. Display size
  (`c`/`r` cells) is unchanged; upload resolution shifts by ≤ half a cell.
  Dead ends tried first: (a) truncating each strip's `x*px/cells` independently —
  up to 1px drift + directional bias, visible seam (first "distortion" report);
  (b) mapping cells to `round(px/cells)` unit blocks *without* a matching upload —
  rounds up for right/bottom strips, pushes source start past ideal, zooms the
  piece ("worse"); (c) shared-rounded `srcX/srcY(cell)` at natural upload size —
  ≤0.5px residual, "almost perfect but not quite". The cell-multiple upload
  required updating `richblocks_test.go` `TestChatViewSizesAttachmentMedia*` /
  `...RespectsMediaProxyQueryDimensions`, which pin upload `s=`/`v=` (now the
  snapped dims, e.g. 400→416, 800→816×804); their intent (upload at metadata /
  original size, display respects fit) is preserved.

## Gotchas / limits

- Occluder is a single bbox: two separate overlays over one image over-hide the
  gap between them. Fine for the real cases (one context menu / profile popup).
- **mpv video is out of scope**: it writes Kitty bytes via `App.WriteRaw`,
  bypassing the buffer, so occlusion can't touch it — but video always plays
  full-screen in a mediaViewer overlay (replaces the tree), so nothing coexists.
- All graphics now standardize on `z=-1` (added `ItemGraphic.Z`, set profile
  avatar/picker thumbnails to -1). Rule: z = below text *within a layer*;
  cross-layer overlap is resolved by occlusion, not z. See [[media-floating-viewer]],
  [[inline-video-mpv]], [[chat-header-and-popup-interactions]].
- A transient popup must not share a Kitty **image ID** with retained chat media.
  Removing the popup emits `Free` (`d=I`) for its image; that deletes every
  placement using the ID, while an unchanged chat graphic emits no replacement.
  The profile avatar therefore namespaces its image ID with `profile:avatar:`
  even when it displays the same URL as the chat author avatar.

Tests: `internal/tui/screen/diff_test.go` (reclip/suppression/ClearAll),
`buffer_test.go` (layer stamps). `go test -race ./internal/tui/... ./internal/ui/...`
passes. Not validated live in sandbox (no Kitty).
