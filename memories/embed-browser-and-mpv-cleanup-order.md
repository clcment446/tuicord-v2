---
name: embed-browser-and-mpv-cleanup-order
summary: Embed thumbnails and web links open in the system browser; only attachment videos use mpv, whose final Kitty delete must drain before the UI force-repaints.
tags: [#embeds, #links, #browser, #video, #mpv, #kitty, #overlay, #media]
impact: critical
date: 2026-07-18
scope: internal/ui/embedview.go, internal/ui/chatview.go, internal/ui/shell.go, internal/markup/parser.go, internal/media/video_linux.go, internal/tui/tui/app.go, internal/tui/term/browser.go
---

## Behavior

Discord video/GIFV embeds are previews, not mpv inputs. Their image/thumbnail is
rendered normally and activation opens `Embed.URL` in the system browser,
falling back to `VideoURL`. Markdown and bare HTTP(S) links emit
`ActionOpenURL` and share the same validated browser launcher. Uploaded video
attachments remain mpv-playable.

Exception: actual GIF URLs and GIFV previews stay in the media viewer rather
than opening their provider link. The viewer initially shows the inline frame,
then refetches from the raw disk cache with no downscale cap and swaps in the
full-resolution image or GIF frames.

## mpv/Kitty details

mpv's Kitty VO emits global `a=d` deletes at startup and shutdown even with
`--vo-kitty-config-clear=no`. The reader must finish queueing the final delete
before its exit callback closes the overlay, and the event loop must drain raw
output after posts but before force-repainting. Reversing that order makes the
delete erase the restored inline images/GIFs until a later redraw.

Pass explicit `--vo-kitty-width/height` from terminal cells times configured
cell pixels. Use `left=0,top=0` so mpv centers the video; otherwise its fallback
pixel area is small and forcing coordinates to 1 pins it at the top-left.
Bound live raw-frame draining so terminal input (especially Esc) cannot starve,
but completely drain the finite shutdown backlog before a forced repaint.

The video overlay is a player surface, not click-to-dismiss. mpv runs with
`--keep-open=yes` and a per-session Unix JSON IPC socket. Overlay ticks query
position, duration, pause, and EOF state; controls issue pause, absolute-percent
seek, and replay commands. Video-body clicks are consumed, the bottom controls
are clickable, and only Esc/q close playback. Manual close stops mpv, queues an
explicit Kitty delete-all, then force-repaints after that delete is flushed so
the final video frame cannot remain over chat.

Keyboard controls are config actions under `[keys]`: `video_pause` (Space),
`video_seek_backward` (Left), `video_seek_forward` (Right), and `video_replay`
(`r`). Seek arrows move five seconds through mpv IPC and are scoped to the video
overlay.

Terminal resize is handled in place: `mediaViewer.Draw` detects a changed cell
size once, posts the work outside drawing, recomputes the padded/control-reserved
region, resizes the PTY, sends SIGWINCH, and updates all `options/vo-kitty-*`
geometry properties through mpv IPC. Playback position and pause state survive.
The mpv PTY region excludes the bottom two terminal rows so its Kitty placement
cannot cover the visible control bar. Pin explicit coordinates: automatic positioning
uses the live terminal cursor, so control redraws otherwise make subsequent
frames jump into the bottom rows. Inset the video and controls by one cell on
every applicable edge.

On local sessions use Kitty shared-memory transfer; SSH retains streamed
payloads. mpv emits each complete `t=s` shared-memory frame with `m=1`. Do not
treat that flag as a streamed continuation: the payload is only the shared
memory object name and must be forwarded immediately. Buffering it while
waiting for `m=0` makes all video disappear.

mpv also reuses the same shared-memory object across frames. Because the app
queues and forwards commands asynchronously, pause/resume can let mpv overwrite
that object before the real terminal reads it, producing a partial main image
and stray strips. Snapshot each `t=s` object to a unique POSIX shared-memory
name, rewrite the Kitty payload, and retain bounded cleanup ownership.

PTY reads are arbitrary byte fragments and may split a Kitty APC payload. Buffer
all `m=1` continuation commands through the final `m=0` command and queue the
whole image transmission as one raw write; otherwise a cell render can land in
the middle and the remaining base64 appears as accumulating random characters.
The terminal write loop must also handle short writes without yielding.
During live playback flush at most one queued frame per event-loop turn and use
a small bounded queue, ensuring input gets a scheduling opportunity between
frames. Shutdown still drains the complete finite backlog before repaint.
