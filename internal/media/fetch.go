package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Doer is the minimal HTTP transport interface used by Fetcher.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

var defaultHTTPClient = &http.Client{
	Timeout: DefaultRequestTimeout,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       60 * time.Second,
	},
}

// Fetcher downloads and decodes media. Zero resource limits are replaced by
// bounded defaults, so a partially initialized Fetcher remains safe.
type Fetcher struct {
	HTTP  Doer
	Cache *Cache

	MaxPixels image.Point

	MaxResponseBytes   int64
	MaxSourcePixels    int64
	MaxSourceDimension int
	GIFMaxFrames       int
	GIFMaxMemoryBytes  int64
	RequestTimeout     time.Duration

	// DisableDiskCache keeps the decoded in-memory LRU but bypasses persistent
	// raw bytes. This is used by the privacy configuration.
	DisableDiskCache bool
}

func (f *Fetcher) limits() Config {
	return Config{
		MaxResponseBytes:   f.MaxResponseBytes,
		MaxSourcePixels:    f.MaxSourcePixels,
		MaxSourceDimension: f.MaxSourceDimension,
		GIFMaxFrames:       f.GIFMaxFrames,
		GIFMaxMemoryBytes:  f.GIFMaxMemoryBytes,
		RequestTimeout:     f.RequestTimeout,
	}.Bounded()
}

// Fetch downloads and decodes a still image.
func (f *Fetcher) Fetch(ctx context.Context, url string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.Cache != nil {
		if img := f.Cache.GetLRU(url); img != nil {
			return img, nil
		}
	}
	raw, cacheable, err := f.getRaw(ctx, url)
	if err != nil {
		return nil, err
	}
	limits := f.limits()
	img, err := DecodeWithLimits(bytes.NewReader(raw), DecodeLimits{
		MaxEncodedBytes: limits.MaxResponseBytes,
		MaxDimension:    limits.MaxSourceDimension,
		MaxPixels:       limits.MaxSourcePixels,
	})
	if err != nil {
		return nil, fmt.Errorf("media: decode %s: %w", url, err)
	}
	if f.MaxPixels.X > 0 && f.MaxPixels.Y > 0 {
		img = DownscaleToPixels(img, f.MaxPixels.X, f.MaxPixels.Y)
	}
	if f.Cache != nil && cacheable {
		f.Cache.PutLRU(url, img)
	}
	return img, nil
}

// FetchGIF downloads and composes all GIF frames within the configured canvas,
// frame-count, and aggregate-memory limits.
func (f *Fetcher) FetchGIF(ctx context.Context, url string) ([]Frame, error) {
	raw, cacheable, err := f.getRaw(ctx, url)
	if err != nil {
		return nil, err
	}
	limits := f.limits()
	frames, err := DecodeGIFWithLimits(bytes.NewReader(raw), GIFLimits{
		DecodeLimits: DecodeLimits{
			MaxEncodedBytes: limits.MaxResponseBytes,
			MaxDimension:    limits.MaxSourceDimension,
			MaxPixels:       limits.MaxSourcePixels,
		},
		MaxFrames:      limits.GIFMaxFrames,
		MaxMemoryBytes: limits.GIFMaxMemoryBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("media: decode gif %s: %w", url, err)
	}
	if f.Cache != nil && cacheable && len(frames) > 0 {
		f.Cache.PutLRU(url, frames[0].Image)
	}
	return frames, nil
}

func (f *Fetcher) getRaw(ctx context.Context, url string) ([]byte, bool, error) {
	limit := f.limits().MaxResponseBytes
	if f.Cache != nil && !f.DisableDiskCache {
		raw, err := f.Cache.GetDiskLimit(url, limit)
		if err == nil && raw != nil {
			if int64(len(raw)) > limit {
				return nil, false, fmt.Errorf("media: cached response for %s is %d bytes, limit is %d", url, len(raw), limit)
			}
			return raw, true, nil
		}
	}
	raw, cacheable, err := f.httpGet(ctx, url)
	if err != nil {
		return nil, false, err
	}
	if f.Cache != nil && !f.DisableDiskCache && cacheable {
		_ = f.Cache.PutDisk(url, raw)
	}
	return raw, cacheable, nil
}

// httpGet bounds both the round-trip duration and response bytes. Content-Length
// is rejected up front when available; a max+1 limited read handles absent or
// dishonest lengths without allocating an unbounded buffer.
func (f *Fetcher) httpGet(ctx context.Context, url string) ([]byte, bool, error) {
	limits := f.limits()
	ctx, cancel := context.WithTimeout(ctx, limits.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("media: build request for %s: %w", url, err)
	}
	doer := Doer(defaultHTTPClient)
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
	if resp.ContentLength > limits.MaxResponseBytes {
		return nil, false, fmt.Errorf("media: response for %s is %d bytes, limit is %d", url, resp.ContentLength, limits.MaxResponseBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limits.MaxResponseBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("media: read body %s: %w", url, err)
	}
	if int64(len(raw)) > limits.MaxResponseBytes {
		return nil, false, fmt.Errorf("media: response for %s exceeds %d bytes", url, limits.MaxResponseBytes)
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
