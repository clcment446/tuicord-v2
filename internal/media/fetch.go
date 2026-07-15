package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
)

// Doer is the minimal HTTP transport interface used by Fetcher. *http.Client
// satisfies it. Inject a stub in tests to avoid real network calls.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Fetcher downloads and decodes media. It consults a two-level Cache before
// making network requests, and stores results in both cache levels on a miss.
//
// # Concurrency contract
//
// Fetch and FetchGIF are synchronous and context-cancellable. They block on
// network I/O and MUST NOT be called from the UI goroutine. Run them on a
// dedicated goroutine and deliver results via app.Post.
type Fetcher struct {
	// HTTP is the transport used for all outbound requests.
	// When nil, http.DefaultClient is used.
	HTTP Doer
	// Cache is the two-level decode cache. When nil, every call performs a
	// network request and results are not stored.
	Cache *Cache
	// MaxPixels bounds the decoded image to at most X×Y pixels, preserving
	// aspect ratio. A zero X or Y disables downscaling.
	//
	// Downscaling here rather than at draw time does the resample once per
	// fetch instead of once per frame, and bounds what the LRU holds. It also
	// keeps the image pointer stable for a warm URL, which matters because the
	// Kitty upload cache keys on that pointer: downscaling after a cache read
	// would mint a new pointer on every fetch and defeat the cache.
	MaxPixels image.Point
}

// Fetch downloads and decodes the image at url. Lookup order:
//  1. In-memory LRU (zero allocations on a warm hit).
//  2. Disk cache (decodes raw bytes, then populates the LRU).
//  3. HTTP GET (populates disk and LRU on success).
//
// The returned image.Image is safe to pass to widget.NewImageFrom.
// Run Fetch on a goroutine; deliver the result via app.Post.
func (f *Fetcher) Fetch(ctx context.Context, url string) (image.Image, error) {
	// 1. LRU hit.
	if f.Cache != nil {
		if img := f.Cache.GetLRU(url); img != nil {
			return img, nil
		}
	}

	// 2 & 3. Get raw bytes (disk → network).
	raw, cacheable, err := f.getRaw(ctx, url)
	if err != nil {
		return nil, err
	}

	img, err := Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("media: decode %s: %w", url, err)
	}
	// Downscale before caching so the LRU holds display-sized images and the
	// cached pointer stays stable across warm hits.
	if f.MaxPixels.X > 0 && f.MaxPixels.Y > 0 {
		img = DownscaleToPixels(img, f.MaxPixels.X, f.MaxPixels.Y)
	}

	if f.Cache != nil && cacheable {
		f.Cache.PutLRU(url, img)
	}
	return img, nil
}

// FetchGIF downloads and decodes all frames of the animated GIF at url.
// The LRU is not used for GIF frame slices (they are too large); raw bytes are
// served from disk when available and decoded fresh on every call. A warm disk
// hit avoids the network round-trip.
//
// Run FetchGIF on a goroutine; deliver the result via app.Post.
func (f *Fetcher) FetchGIF(ctx context.Context, url string) ([]Frame, error) {
	raw, cacheable, err := f.getRaw(ctx, url)
	if err != nil {
		return nil, err
	}
	frames, err := DecodeGIF(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("media: decode gif %s: %w", url, err)
	}
	// Store first frame in LRU as a thumbnail.
	if f.Cache != nil && cacheable && len(frames) > 0 {
		f.Cache.PutLRU(url, frames[0].Image)
	}
	return frames, nil
}

// getRaw returns the raw bytes for url, consulting the disk cache before
// making a network request. On a network hit it writes the bytes to disk.
func (f *Fetcher) getRaw(ctx context.Context, url string) ([]byte, bool, error) {
	// Disk hit: no network needed.
	if f.Cache != nil {
		raw, err := f.Cache.GetDisk(url)
		if err == nil && raw != nil {
			return raw, true, nil
		}
	}

	raw, cacheable, err := f.httpGet(ctx, url)
	if err != nil {
		return nil, false, err
	}

	// Populate disk cache (best-effort — write errors are non-fatal).
	if f.Cache != nil && cacheable {
		_ = f.Cache.PutDisk(url, raw)
	}
	return raw, cacheable, nil
}

// httpGet performs an HTTP GET for url and returns the response body.
// It honours ctx cancellation throughout the round-trip.
func (f *Fetcher) httpGet(ctx context.Context, url string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("media: build request for %s: %w", url, err)
	}

	doer := Doer(http.DefaultClient)
	if f.HTTP != nil {
		doer = f.HTTP
	}

	resp, err := doer.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("media: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("media: GET %s: unexpected status %d", url, resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("media: read body %s: %w", url, err)
	}
	return raw, responseCacheable(resp.Header), nil
}

func responseCacheable(header http.Header) bool {
	for _, directive := range strings.Split(strings.ToLower(header.Get("Cache-Control")), ",") {
		directive = strings.TrimSpace(directive)
		if directive == "no-store" || directive == "no-cache" || directive == "max-age=0" {
			return false
		}
	}
	return true
}
