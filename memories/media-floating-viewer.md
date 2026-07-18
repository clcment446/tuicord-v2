---
name: media-floating-viewer
summary: A full-screen mediaViewer overlay shows images/GIFs enlarged (our own Kitty placement) and hosts full-screen mpv video; loading media reserves its final height so async loads don't shift the viewport.
tags: [#chat, #media, #viewer, #overlay, #video, #images, #scroll]
impact: high
commit: 98d8609 (dirty)
date: 2026-07-18
created_at: 2026-07-18T01:30:00+02:00
scope: internal/ui/media_viewer.go, internal/ui/chatview.go, internal/ui/shell.go
---

## What

`mediaViewer` (`internal/ui/media_viewer.go`) is a full-screen overlay
(`Shell.overlay`, so it replaces the chat) with two modes:
- **image/GIF**: draws the frame centered via our own `widget.NewKittyImageFrom`
  (we control placement, so no mpv coordinate problems). Image ID is namespaced
  `stableImageID("mediaviewer:"+key)` so it never collides with the inline copy.
  Resolution is the inline-downscaled image (limited); full-res would need a
  no-downscale refetch. GIFs show a static current frame (no animation yet).
- **video**: blank black backdrop; mpv paints frames over it (see
  [[inline-video-mpv]]).

## Triggers & wiring

- `ChatView.onOpenMedia(url, img)` → `Shell.openMediaViewer`. `onPlayVideo` →
  `Shell.playVideo` (full-screen mpv). Both set as callbacks in `NewShell`.
- Activation: click an image/GIF block (`activateAt` image branch) or press `o`
  on the focused message (`openFocusedMedia`: video → player, else first loaded
  image). `p` still plays the focused video. Esc closes via the Shell overlay
  layer (`closeOverlay`), `q`/click via `mediaViewer.Handle`.

## Related fixes (same batch)

- **Loading no longer shifts the viewport**: `loadingPlaceholderLines` /
  `reservedMediaRows` reserve the loaded image's exact row count (from the media's
  known dimensions via `fitMediaCells`) while it loads, so the async swap is
  in-place. Falls back to one spinner line when dimensions are unknown.
- **Loaded media / GIF start appear immediately**: `ChatView.SetInvalidate`
  (wired to `App.Invalidate` in main.go) is called from the fetch posts, so media
  shows on the next loop turn instead of waiting for the ~500ms idle tick.
- **Framed video embeds play**: `embedPlayableMedia` feeds the framed embed path
  (was `mediaLines`, poster-only) so video embeds with a title/provider are
  playable, not just unadorned ones.

Tests: `internal/ui/media_viewer_test.go`, `media_layout_test.go`. Not validated
live (no Kitty terminal/mpv in sandbox).