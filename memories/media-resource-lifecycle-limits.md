---
name: media-resource-lifecycle-limits
summary: Media bytes, source geometry, GIF composition, cache, clipboard extraction, async fetches, raw Kitty output, and Shell shutdown all require independent bounds and shared cancellation.
tags: [#media, #gif, #cache, #clipboard, #lifecycle, #mpv, #kitty, #privacy]
impact: critical
date: 2026-07-18
scope: internal/media, internal/ui, internal/tui/term, internal/tui/tui/app.go, internal/config/config.go, cmd/tuicord/main.go
---

## Resource model

HTTP media rejects oversized `Content-Length` and also reads through a max+1
limited reader. A request timeout is applied through context even when callers
supply a background context. Still images run `DecodeConfig` and validate source
dimensions/pixels before full decode. GIF encoded blocks are scanned before
`gif.DecodeAll`: canvas/frame bounds and aggregate composed memory are checked,
and the stream is terminated after the configured first-N frame cap so excess
frames are never decoded or composed.

The raw disk cache is atomic, TTL bounded, byte bounded, and prunes oldest files.
Persistent caching and idle prefetch are independently disableable through
privacy config while retaining an in-memory decoded LRU.

## UI lifecycle

Chat media uses a fixed worker pool and bounded queue; saturation creates no
semaphore-waiting goroutine and retries on a later render. Main chat, forum
preview, picker thumbnails, prefetch, and full-resolution viewer requests all
have cancellation. Viewer limits are separately configurable and larger than
inline limits while remaining finite. `Shell.Close` cancels all of these,
cancels clipboard extraction, and kills/waits briefly for the mpv process group;
main always defers it.

Clipboard image tools run with context, timeout, and output caps on a background
goroutine. `App.WriteRaw` queues copied, complete Kitty transmissions without
blocking; its small queue drops only whole oldest transmissions and ignores
writes after loop shutdown. This retains the required final-delete-before-force-
repaint order.

## Preserved behavior

GIF animation remains client-driven and visible-only. GIFs over the frame cap
animate their first bounded frame set. GIFV stays in the image viewer. Uploaded
video remains full-screen mpv with controls, shared-memory stabilization, and
complete shutdown draining. Actionable notification activation closes popups,
viewers, and video before channel navigation.

Targeted media/UI/TUI race tests and root `go test -race ./...` pass.

## Follow-up lifecycle findings

The decoded LRU needs an independent byte budget in addition to its 64-entry
cap; GIF fallback entries must be downscaled before insertion (with oversized
single entries skipped). When persistent media is private/disabled, construct a
memory-only cache directly: even constructing the disk-backed cache performs
cache-dir resolution, stat, and pruning IO.

Event-loop exit owns terminal ordering. It must close the root while the raw
writer is live, let Shell cancellation join media/prefetch/viewer/clipboard/mpv
workers, reject new posts, drain already accepted shutdown-aware closures, and
flush the finite final Kitty delete backlog before terminal restoration closes
the writer. Async temp-file producers use `TryPost`; a rejected post and an
accepted closure that later observes shutdown both delete the unowned file.

Clipboard operation deadlines are UI completions, not lifecycle exits: marshal
them back to clear busy/cancel state and show a timeout, while lifecycle
cancellation stays silent. Protect the state so rejected posts and `Close` can
both clear it without racing.

mpv SHM snapshots need a per-frame byte cap and an independent total retained
byte budget. Copy source files with bounded IO, remove partial destinations on
all errors, and account snapshot sizes during eviction and final cleanup. Since
the Kitty framer has already isolated a complete SHM APC command, an oversized
or unreadable frame must be dropped as a whole command; forwarding part of it
would leave later terminal output inside Kitty's payload stream.
