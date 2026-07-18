package ui

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awesomeProject/internal/config"
	"awesomeProject/internal/media"
)

type blockingMediaDoer struct {
	started  chan struct{}
	canceled chan struct{}
	calls    atomic.Int32
}

func (d *blockingMediaDoer) Do(req *http.Request) (*http.Response, error) {
	d.calls.Add(1)
	select {
	case d.started <- struct{}{}:
	default:
	}
	<-req.Context().Done()
	select {
	case <-d.canceled:
	default:
		close(d.canceled)
	}
	return nil, req.Context().Err()
}

func TestChatMediaQueueIsBoundedAndCloseCancelsFetch(t *testing.T) {
	doer := &blockingMediaDoer{started: make(chan struct{}, 1), canceled: make(chan struct{})}
	view := NewChatView(nil, nil, nil, Styles{})
	cfg := media.DefaultConfig()
	cfg.ConcurrentFetches = 1
	cfg.QueuedFetches = 1
	cfg.RequestTimeout = time.Minute
	view.SetMedia(&media.Fetcher{HTTP: doer, RequestTimeout: time.Minute}, cfg, func(func()) {})

	if state := view.ensureMedia("https://example.com/one.png", false); state == nil {
		t.Fatal("first media job was not accepted")
	}
	select {
	case <-doer.started:
	case <-time.After(time.Second):
		t.Fatal("first media job did not start")
	}
	if state := view.ensureMedia("https://example.com/two.png", false); state == nil {
		t.Fatal("queued media job was not accepted")
	}
	if state := view.ensureMedia("https://example.com/three.png", false); state != nil {
		t.Fatal("saturated queue accepted a third job")
	}

	view.CloseMedia()
	if got := doer.calls.Load(); got != 1 {
		t.Fatalf("worker started queued work after cancellation: calls=%d", got)
	}
	select {
	case <-doer.canceled:
	case <-time.After(time.Second):
		t.Fatal("CloseMedia did not cancel the in-flight request")
	}
}

func TestPrefetchCloseCancelsAndJoinsWorker(t *testing.T) {
	doer := &blockingMediaDoer{started: make(chan struct{}, 1), canceled: make(chan struct{})}
	prefetch := newIdleMediaPrefetcher(&media.Fetcher{HTTP: doer, RequestTimeout: time.Minute})
	prefetch.Start([]string{"https://example.com/one.png", "https://example.com/two.png"})
	select {
	case <-doer.started:
	case <-time.After(time.Second):
		t.Fatal("prefetch did not start")
	}
	prefetch.Close()
	select {
	case <-doer.canceled:
	default:
		t.Fatal("Close returned before the prefetch request observed cancellation")
	}
	if got := doer.calls.Load(); got != 1 {
		t.Fatalf("prefetch started queued work after close: calls=%d", got)
	}
}

func TestDisabledPersistentCacheConstructsMemoryOnlyAndDoesNotPrune(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	dir := filepath.Join(cacheRoot, "tuicord", "media")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, strings.Repeat("a", 64))
	if err := os.WriteFile(stale, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	appCfg := config.Default()
	appCfg.Privacy.PersistMediaCache = false
	fetcher := newChatMediaFetcher(chatMediaConfig(appCfg))
	if fetcher == nil || fetcher.Cache == nil {
		t.Fatal("memory-only fetcher was not constructed")
	}
	if fetcher.Cache.Dir != "" || !fetcher.DisableDiskCache {
		t.Fatalf("persistent cache was constructed: dir=%q disabled=%v", fetcher.Cache.Dir, fetcher.DisableDiskCache)
	}
	if got, err := os.ReadFile(stale); err != nil || string(got) != "private" {
		t.Fatalf("privacy-disabled construction touched cache file: %q, %v", got, err)
	}
}

func TestMediaPrivacyConfigWiresAllExternalFeatures(t *testing.T) {
	cfg := config.Default()
	cfg.Privacy.FetchExternalMedia = false
	cfg.Privacy.PersistMediaCache = false
	cfg.Privacy.PrefetchMedia = false
	cfg.Privacy.PlayVideos = false
	got := chatMediaConfig(cfg)
	if got.Enabled || got.DiskCacheEnabled || got.Prefetch || got.VideoEnabled {
		t.Fatalf("privacy settings not projected: %+v", got)
	}

	cfg = config.Default()
	cfg.Media.ViewerMaxResponseBytes = 1234
	cfg.Media.ViewerMaxSourcePixels = 5678
	viewer := viewerMediaConfig(cfg)
	if viewer.MaxResponseBytes != 1234 || viewer.MaxSourcePixels != 5678 {
		t.Fatalf("viewer limits = %d/%d", viewer.MaxResponseBytes, viewer.MaxSourcePixels)
	}
}

func TestShellCloseCancelsLifecycleOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sh := &Shell{lifecycleCtx: ctx, lifecycleCancel: cancel}
	sh.Close()
	sh.Close()
	if ctx.Err() == nil {
		t.Fatal("Shell.Close did not cancel lifecycle context")
	}
}
