// Package media fetches, decodes, and caches Discord media — images, GIFs,
// stickers, and emojis.
//
// # Design contract
//
// All public functions in this package are synchronous and context-cancellable.
// They block on network I/O and must never be called from the UI goroutine.
// Callers are responsible for running Fetch / FetchGIF on a goroutine and
// delivering results back through app.Post (the established tuicord discipline
// for all off-thread work). The package itself has no knowledge of app.Post —
// it is a pure decode/cache/classify layer.
//
// # Classification
//
// ClassifyURL is a pure, allocation-efficient function that inspects a URL and
// returns a Class constant. It is the single source of truth for distinguishing
// Discord sticker links, emoji CDN links, plain images, GIFs, and videos —
// including the "fake-nitro" patterns where users share bare CDN URLs.
//
// # Caching
//
// Cache implements a two-level strategy: an in-memory LRU (decoded image.Image
// values, bounded to a configurable entry count) backed by a disk store (raw
// encoded bytes under ~/.cache/tuicord/media/<sha256(url)>). On a warm LRU hit
// no allocations occur beyond the return path. On a disk hit the image is
// decoded once and inserted into the LRU. Network requests are only made when
// both cache levels miss.
//
// # Animated GIFs
//
// DecodeGIF returns fully composed frames (prior frames are composited per the
// GIF disposal method so each Frame.Image is a complete picture, not a delta).
// The Fetcher does not LRU animated frames — callers re-decode from the disk
// cache. First-frame thumbnails of GIFs are stored in the LRU like any still
// image.
package media
