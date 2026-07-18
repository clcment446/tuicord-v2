package media

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"net/http"
	"testing"
	"time"
)

// stubDoer is a fake HTTP transport that always returns the same body.
type stubDoer struct {
	status        int
	body          []byte
	header        http.Header
	contentLength int64
	calls         int
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	s.calls++
	return &http.Response{
		StatusCode:    s.status,
		Header:        s.header,
		Body:          io.NopCloser(bytes.NewReader(s.body)),
		ContentLength: s.contentLength,
	}, nil
}

func TestFetcherHonorsNoStoreResponsePolicy(t *testing.T) {
	raw := makePNG(t, 1, 1, color.RGBA{A: 255})
	stub := &stubDoer{status: http.StatusOK, body: raw, header: http.Header{"Cache-Control": []string{"no-store"}}}
	f := &Fetcher{HTTP: stub, Cache: newTempCache(t, 4)}
	for i := 0; i < 2; i++ {
		if _, err := f.Fetch(context.Background(), "https://example.com/no-store.png"); err != nil {
			t.Fatal(err)
		}
	}
	if stub.calls != 2 {
		t.Fatalf("no-store requests = %d, want 2", stub.calls)
	}
}

// newStubFetcher returns a Fetcher backed by a fake transport and a temp-dir
// Cache. The supplied raw bytes are what the fake HTTP "server" returns.
func newStubFetcher(t *testing.T, raw []byte) (*Fetcher, *stubDoer) {
	t.Helper()
	stub := &stubDoer{status: http.StatusOK, body: raw}
	c := newTempCache(t, 8)
	return &Fetcher{HTTP: stub, Cache: c}, stub
}

func TestFetcher_Fetch_DecodesImage(t *testing.T) {
	// Arrange: a valid PNG served by the fake transport.
	raw := makePNG(t, 6, 4, color.RGBA{R: 100, G: 150, B: 200, A: 255})
	f, _ := newStubFetcher(t, raw)
	url := "https://example.com/img.png"
	// Act.
	img, err := f.Fetch(context.Background(), url)
	// Assert.
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 6 || b.Dy() != 4 {
		t.Errorf("Fetch: image bounds = %v, want 6×4", b)
	}
}

func TestFetcher_Fetch_LRUHitSkipsNetwork(t *testing.T) {
	// Arrange: first fetch warms the LRU.
	raw := makePNG(t, 2, 2, color.RGBA{A: 255})
	f, stub := newStubFetcher(t, raw)
	url := "https://example.com/img.png"
	if _, err := f.Fetch(context.Background(), url); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	firstCalls := stub.calls
	// Act: second fetch must hit the LRU.
	if _, err := f.Fetch(context.Background(), url); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	// Assert: no new HTTP calls after the LRU was warmed.
	if stub.calls != firstCalls {
		t.Errorf("expected LRU hit to skip network; calls went from %d to %d", firstCalls, stub.calls)
	}
}

func TestFetcher_Fetch_DiskHitSkipsNetwork(t *testing.T) {
	// Arrange: warm the disk cache manually, then construct a fetcher with a
	// fresh LRU (simulates a process restart).
	raw := makePNG(t, 2, 2, color.RGBA{G: 200, A: 255})
	url := "https://example.com/disk.png"
	c := newTempCache(t, 8)
	if err := c.PutDisk(url, raw); err != nil {
		t.Fatalf("PutDisk: %v", err)
	}
	stub := &stubDoer{status: http.StatusOK, body: raw}
	f := &Fetcher{HTTP: stub, Cache: c}
	// Act.
	if _, err := f.Fetch(context.Background(), url); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Assert: disk hit → no network call.
	if stub.calls != 0 {
		t.Errorf("expected disk hit to skip network; got %d HTTP calls", stub.calls)
	}
}

func TestFetcher_Fetch_PopulatesDiskCache(t *testing.T) {
	// Arrange.
	raw := makePNG(t, 3, 3, color.RGBA{B: 128, A: 255})
	f, _ := newStubFetcher(t, raw)
	url := "https://example.com/populate.png"
	// Act.
	if _, err := f.Fetch(context.Background(), url); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Assert: raw bytes written to disk.
	got, err := f.Cache.GetDisk(url)
	if err != nil {
		t.Fatalf("GetDisk: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("disk cache: got %d bytes, want %d", len(got), len(raw))
	}
}

func TestFetcher_FetchGIF_ReturnsFrames(t *testing.T) {
	// Arrange: a 3-frame GIF.
	raw := makeGIF(t, 8, 8, 3, 10)
	f, _ := newStubFetcher(t, raw)
	url := "https://example.com/anim.gif"
	// Act.
	frames, err := f.FetchGIF(context.Background(), url)
	// Assert.
	if err != nil {
		t.Fatalf("FetchGIF: %v", err)
	}
	if len(frames) != 3 {
		t.Errorf("FetchGIF: got %d frames, want 3", len(frames))
	}
}

func TestFetcher_Fetch_HTTPError(t *testing.T) {
	// Arrange: stub returns 404.
	stub := &stubDoer{status: http.StatusNotFound, body: []byte("not found")}
	c := newTempCache(t, 4)
	f := &Fetcher{HTTP: stub, Cache: c}
	// Act.
	_, err := f.Fetch(context.Background(), "https://example.com/missing.png")
	// Assert: must surface the error.
	if err == nil {
		t.Fatal("Fetch with 404: expected error, got nil")
	}
}

func TestFetcher_Fetch_NilCache(t *testing.T) {
	// Arrange: no cache at all — every call hits the network.
	raw := makePNG(t, 1, 1, color.RGBA{A: 255})
	stub := &stubDoer{status: http.StatusOK, body: raw}
	f := &Fetcher{HTTP: stub, Cache: nil}
	url := "https://example.com/nocache.png"
	// Act.
	img, err := f.Fetch(context.Background(), url)
	// Assert.
	if err != nil {
		t.Fatalf("Fetch without cache: %v", err)
	}
	if img == nil {
		t.Fatal("Fetch without cache: got nil image")
	}
}

func TestFetcherRejectsContentLengthBeforeRead(t *testing.T) {
	stub := &stubDoer{status: http.StatusOK, contentLength: 9, body: []byte("x")}
	f := &Fetcher{HTTP: stub, MaxResponseBytes: 8}
	if _, err := f.Fetch(context.Background(), "https://example.com/large.png"); err == nil {
		t.Fatal("Fetch accepted oversized Content-Length")
	}
}

func TestFetcherRejectsChunkedBodyAtMaxPlusOne(t *testing.T) {
	stub := &stubDoer{status: http.StatusOK, contentLength: -1, body: []byte("123456789")}
	f := &Fetcher{HTTP: stub, MaxResponseBytes: 8}
	if _, err := f.Fetch(context.Background(), "https://example.com/large.png"); err == nil {
		t.Fatal("Fetch accepted oversized body without Content-Length")
	}
}

func TestFetcherRequestTimeoutCancelsTransport(t *testing.T) {
	f := &Fetcher{HTTP: doerFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}), RequestTimeout: 10 * time.Millisecond}
	if _, err := f.Fetch(context.Background(), "https://example.com/slow.png"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Fetch error = %v, want deadline exceeded", err)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestFetcher_Fetch_ContextCancelled(t *testing.T) {
	// Arrange: immediately cancelled context.
	raw := makePNG(t, 1, 1, color.RGBA{A: 255})
	stub := &stubDoer{status: http.StatusOK, body: raw}
	f := &Fetcher{HTTP: stub, Cache: nil}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before making any request
	// Act.
	// The stubDoer doesn't honour ctx, but Fetcher constructs the request with
	// ctx so a real transport would cancel. Here we just check no panic.
	_, _ = f.Fetch(ctx, "https://example.com/img.png")
}
