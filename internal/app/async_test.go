package app

import (
	"errors"
	"testing"
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
