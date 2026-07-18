package ui

import (
	"context"
	"net/http"
	"testing"
	"time"

	"awesomeProject/internal/config"
	"awesomeProject/internal/media"
)

type blockingMediaDoer struct {
	started  chan struct{}
	canceled chan struct{}
}

func (d *blockingMediaDoer) Do(req *http.Request) (*http.Response, error) {
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
	select {
	case <-doer.canceled:
	case <-time.After(time.Second):
		t.Fatal("CloseMedia did not cancel the in-flight request")
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
