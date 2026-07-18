---
name: inline-gif-animation
summary: Inline GIFs animate client-side by advancing decoded frames on a wall-clock tick and re-uploading each frame through the existing Kitty path; the runtime raises tick cadence only while a GIF is visible.
tags: [#chat, #media, #gif, #animation, #kitty, #tick, #runtime]
impact: high
commit: 98d8609 (dirty)
date: 2026-07-18
created_at: 2026-07-18T00:00:00+02:00
scope: internal/ui/chatview.go, internal/ui/richblocks.go, internal/ui/embedview.go, internal/tui/tui/app.go, internal/ui/shell.go
---

## Why this shape

GIF decode/fetch already existed (`media.DecodeGIF`/`Fetcher.FetchGIF`/`[]media.Frame`
with per-frame delays, `Config.Animate`) but nothing in the UI called it — GIFs
rendered as a static first frame. Animation is now **client-driven** (not the
Kitty native animation protocol) so it works on any terminal the basic graphics
protocol already targets and reuses the existing single-image placement path.

## Key points

- **No widget change.** `drawInlineMedia` re-places `NewKittyImageFrom(block.img)`
  every draw; the Kitty upload cache keys on the image pointer (`image.go:405`,
  `%p`), so a new frame image yields a fresh `Graphic.PayloadKey` under the stable
  placement `Key` → the diff re-transmits the frame under the same image id.
- **Frame state** lives on `chatMediaState` (`frames`, `frameIdx`, `frameElapsed`,
  `lastTick`); `animated()` = `len(frames) > 1`. `mediaVariant` refreshes
  `variant.img = state.img` on cache hit so sizing stays cached while the frame
  advances. `fetchMedia(url, animated)` calls `FetchGIF` when
  `animated && Config.Animate`; `downscaleFrames` applies the fetcher's MaxPixels
  (FetchGIF, unlike Fetch, does not downscale — else every frame re-uploads full size).
- **Animation decision is threaded, not inferred from URL.** `mediaLines`/`ensureMedia`
  take `animated bool`: attachments via `attachmentAnimated` (content-type image/gif
  or `ClassifyURL==ClassGIF`), stickers via `Format==StickerGIF`, embeds via
  `ClassifyURL==ClassGIF`. Emoji pass false (kept static; sticker CDN URLs classify
  as ClassSticker and emoji CDN as ClassEmoji, so URL-only detection would miss them
  anyway). tenor/giphy **gifv** embeds are mp4 (video path), not this GIF path.
- **Tick cadence is adaptive.** The app ticker is 500ms; `advanceFrames` steps by
  real elapsed wall-clock time (a >1s gap resyncs without burst-advancing). The run
  loop (`app.go`) queries `Animator.Animating()` after each render and
  `ticker.Reset`s to `animationTickInterval` (50ms) only while a GIF is visible.
  `Shell.Animating()` → `ChatView.Animating()` returns `animatedVisible`, set in
  `Draw` like `spinnerVisible`. First animated frame can lag up to 500ms after load
  (the spinner-driven redraw sets `animatedVisible`, then the fast ticker engages).
- **Body cache:** `storeBody` skips bodies whose media is `loading` **or**
  `animated()`, mirroring the spinner rule, so frames aren't frozen by memoization.
- Tests: `internal/ui/animation_test.go`. Live GIF playback (Kitty terminal +
  Discord session) was not runnable in the sandbox.

Video (mpv `--vo=kitty` on a PTY, select-to-play) is the separate, unstarted
half of this feature — see [[embed-media-user-gateway]] and the plan at
`~/.claude/plans/plant-the-support-for-structured-key.md`.
