package app

import (
	"errors"
	"testing"
	"time"
)

func TestRunInBackgroundReportsOperationErrors(t *testing.T) {
	want := errors.New("request failed")
	reported := make(chan error, 1)
	a := &App{ui: syncPoster{}}
	a.OnError(func(err error) { reported <- err })

	a.runInBackground(func() error { return want })

	if got := <-reported; !errors.Is(got, want) {
		t.Fatalf("reported error = %v, want %v", got, want)
	}
}

func TestReportErrorOnUICallsHandlerWithoutPostingAgain(t *testing.T) {
	want := errors.New("request failed")
	ui := &queuedPoster{}
	reported := make(chan error, 1)
	a := &App{ui: ui}
	a.OnError(func(err error) { reported <- err })

	a.reportErrorOnUI(want)

	if len(ui.posts) != 0 {
		t.Fatalf("queued posts = %d, want 0", len(ui.posts))
	}
	if got := <-reported; !errors.Is(got, want) {
		t.Fatalf("reported error = %v, want %v", got, want)
	}
}

func TestRunMutationPostsSuccessfulUIUpdate(t *testing.T) {
	completed := make(chan struct{})
	a := &App{ui: syncPoster{}}

	a.runMutation(func() error { return nil }, func() { close(completed) })

	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for successful UI update")
	}
}

func TestPostIfCurrentSkipsStaleCompletion(t *testing.T) {
	ui := &queuedPoster{}
	a := &App{ui: ui}
	completed := false

	a.postIfCurrent(func() bool { return false }, func() { completed = true })
	ui.run()

	if completed {
		t.Fatal("stale completion ran")
	}
}
