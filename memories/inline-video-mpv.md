---
name: inline-video-mpv
summary: Videos play inline (select-to-play, one at a time) by running mpv --vo=kitty on a pty; mpv's graphics bytes are forwarded to the terminal via App.WriteRaw, pinned to the poster's cell rect with --vo-kitty-left/top.
tags: [#chat, #media, #video, #mpv, #kitty, #pty, #runtime]
impact: high
commit: 98d8609 (dirty)
date: 2026-07-18
created_at: 2026-07-18T00:30:00+02:00
scope: internal/media/video_linux.go, internal/media/pty_linux.go, internal/media/video_common.go, internal/ui/chatview.go, internal/ui/shell.go, internal/tui/tui/app.go
---

## Why this shape

Terminals/pure-Go can't decode mp4/webm/mov, so inline video drives **mpv** as a
subprocess and forwards its Kitty-graphics output into our screen. User chose
mpv-on-pty + select-to-play (not autoplay, not an external window). Video and GIF
are separate pipelines: GIF is pure-Go frame animation ([[inline-gif-animation]]),
video is this mpv path.

## Key points

- **Player** (`media.VideoPlayer`, `video_linux.go`; `video_other.go` stubs
  non-Linux). `Play(url, Rect, out, onExit)` opens a pty (`pty_linux.go`, built
  from `x/sys/unix` ioctls — no new dep), launches
  `mpv --vo=kitty --vo-kitty-alt-screen=no --vo-kitty-cols/rows/left/top=... [--no-audio]`
  with `TERM=xterm-kitty`, and forwards pty-master bytes to `out`. `--vo-kitty-left/top`
  are **1-based**; our `media.Rect` is 0-based (`X+1`). One video at a time; `Stop`
  kills mpv + closes master. `Available()` gates on mpv being on PATH.
- **Output seam:** `tui.App.WriteRaw([]byte)` queues bytes on a buffered channel
  the run loop drains **between frame renders** (same goroutine as the cell diff),
  so mpv bytes never interleave mid-escape. `app.App.WriteRaw`/`Invalidate` forward
  to it; the `poster` interface gained both (fakes updated).
- **Poster + activation** (chatview.go): `inlineMedia.videoURL` marks a play
  target; video embeds render the thumbnail poster + a `▶` overlay (drawn as a
  text cell over the z=-1 image), video attachments get a placeholder box (no
  poster URL exists). `mediaLinesVideo`/`videoPlaceholderLines` build them.
  Activation: click (`activateAt` tests `videoHits`) or focused-message `p` key.
  `Esc` / clicking the playing block stops.
- **Region math:** `videoHit` stores chat-local x/y/cols/rows; Draw records
  `chatOriginX/Y = r.Bounds()` (absolute buffer=terminal cells); play region =
  origin + local. `playFocusedVideo` matches a message's blocks via
  `HasPrefix(placementKey, messagePlacementPrefix(m)+":")`.
- **Region hand-off:** while `playingVideo == url`, Draw blanks that block (no
  AddGraphic) so `GraphicDiff` clears the poster placement and mpv owns the cells.
  On stop, `Shell.teardownVideo` writes `media.KittyClearRegion(rect)` (per-cell
  `a=d,d=p` deletes) to erase mpv's final frame (mpv runs alt-screen=no so it
  lingers), then Invalidate re-renders the poster.
- **Stop conditions:** `stopVideoOnLayoutChange` (Draw) stops on channel switch,
  resize (width), or scroll-offset change vs the snapshot taken in
  `SetPlayingVideo`; also `;quit`. Single teardown path: widget events and mpv's
  own exit both funnel through `stopVideoRequest` → `onStopVideo`.
- **Config** (`media.Config`): `MpvPath` (default "mpv"), `VideoAudio` (default
  off — quiet chat), `VideoUseSHM` (default off — more reliable through the pty).
  Replaced the old unused `VideoPlayer` field.

## Playback is full-screen, not in-place (mpv limitation)

True in-place inline video is NOT feasible with mpv: mpv's kitty vo **clears its
whole pty each frame** (a full-size pty → whole screen goes black) and its output
carries mpv's own cursor coordinates, which are not translated when we forward
the bytes (a region-size pty → mpv draws at the screen's top-left, not the
message). So video plays **full-screen in an overlay** (`mediaViewer`,
`internal/ui/media_viewer.go`), where a full-terminal pty is origin-aligned and
mpv's black backdrop reads as the intended player. The inline poster + ▶ is only
the trigger. See [[media-floating-viewer]].

- `Shell.playVideo` opens a `mediaViewer` overlay and plays with `region =` full
  terminal (`term.ProbeSize`). `closeOverlay` (Esc) / `q` / click / mpv-exit all
  run `teardownVideo` → `KittyDeleteAllImages()` (`a=d,d=A`, one escape) then
  `App.ForceRepaint()` (new: discards the diff baseline so the widget tree's own
  images re-upload). `Play` sizes the pty to the region.
- **GIFV (tenor/giphy) is NOT video here** — it animates as a GIF from its
  thumbnail ([[inline-gif-animation]]). Only `EmbedVideo` (via `embedIsVideo`) and
  video attachments are mpv play targets. Routing GIFV to mpv was a regression
  (clicking a tenor gif launched a fullscreen-black player).

The inline plumbing (`SetPlayingVideo`, region blanking, `stopVideoOnLayoutChange`)
still exists but is dormant — kept in case in-place playback becomes feasible.

## Not validated live

Built without a Kitty terminal + mpv in the sandbox; unit-tested only
(`internal/ui/video_test.go`, `media_viewer_test.go`, `internal/media/video_common_test.go`).
