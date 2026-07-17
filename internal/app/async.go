package app

// runInBackground executes a Discord mutation away from the UI goroutine and
// routes its error through the application's standard error callback.
func (a *App) runInBackground(operation func() error) {
	if a == nil || operation == nil {
		return
	}
	go func() {
		a.reportError(operation())
	}()
}
