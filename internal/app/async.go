package app

// runInBackground executes a Discord mutation away from the UI goroutine and
// routes its error through the application's standard error callback.
func (a *App) runInBackground(operation func() error) {
	if a == nil || operation == nil {
		return
	}
	go func() {
		a.reportAsyncError(operation())
	}()
}

// runMutation executes a REST mutation away from the UI goroutine and applies
// its successful local-store update on the UI goroutine. Keeping this sequence
// in one place prevents mutations from drifting on error or shutdown behavior.
func (a *App) runMutation(operation func() error, onSuccessUI func()) {
	if a == nil || operation == nil {
		return
	}
	go func() {
		if err := operation(); err != nil {
			a.reportAsyncError(err)
			return
		}
		if onSuccessUI != nil {
			a.Post(onSuccessUI)
		}
	}()
}

// reportAsyncError schedules an error notification from a background worker.
func (a *App) reportAsyncError(err error) {
	if err != nil {
		a.Post(func() { a.reportErrorOnUI(err) })
	}
}

// reportErrorOnUI reports an error while already running on the UI goroutine.
// It intentionally never posts again, preserving completion ordering.
func (a *App) reportErrorOnUI(err error) {
	if a == nil || err == nil {
		return
	}
	if a.onError != nil {
		a.onError(err)
	}
	a.emit("error", map[string]any{"message": err.Error()})
}

// postIfCurrent applies a background completion only while its captured
// resource lifetime is still current on the UI goroutine. Resource-specific
// snapshot and merge rules stay with their loaders; this owns the common
// scheduling and stale-completion boundary.
func (a *App) postIfCurrent(current func() bool, completion func()) {
	if a == nil || completion == nil {
		return
	}
	a.Post(func() {
		if current == nil || current() {
			completion()
		}
	})
}
