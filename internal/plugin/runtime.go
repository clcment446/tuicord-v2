package plugin

import (
	"context"
	"sync"
)

// runtime owns the single goroutine on which all Lua executes. Work is a plain
// closure; callers use submit for fire-and-forget dispatch (events, key/command
// handlers) and do for load-time work that must complete before proceeding.
type runtime struct {
	jobs   chan func()
	quit   chan struct{}
	wg     sync.WaitGroup
	closed chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once
	// submitMu makes submit and the start of stop mutually exclusive. Without
	// it, submit could observe an open quit channel, then enqueue successfully
	// after stop closed quit and the worker had decided to exit.
	submitMu sync.RWMutex
}

// newRuntime creates a runtime whose job queue holds queue pending jobs before
// submit starts dropping. start must be called to spin up the goroutine.
func newRuntime(queue int) *runtime {
	if queue < 1 {
		queue = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &runtime{
		jobs:   make(chan func(), queue),
		quit:   make(chan struct{}),
		closed: make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
}

// start launches the worker goroutine. It drains the job queue until stop is
// called, running each job to completion in submission order.
func (r *runtime) start() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer close(r.closed)
		for {
			// Prefer shutdown over queued work. The second quit case closes the
			// small race between this check and waiting for the next job.
			select {
			case <-r.quit:
				return
			default:
			}
			select {
			case <-r.quit:
				return
			case job := <-r.jobs:
				job()
			}
		}
	}()
}

// submit enqueues a job without blocking. It reports false if the queue is full
// (the job is dropped) or the runtime is stopping, so a stuck plugin cannot
// back-pressure the gateway or UI goroutines that emit events.
func (r *runtime) submit(job func()) bool {
	if job == nil {
		return false
	}
	r.submitMu.RLock()
	defer r.submitMu.RUnlock()
	select {
	case <-r.quit:
		return false
	default:
	}
	select {
	case r.jobs <- job:
		return true
	default:
		return false
	}
}

// do runs job on the plugin goroutine and blocks until it finishes. It is used
// for loading, where errors must be observed before the next plugin runs. It
// returns false if the runtime is stopping.
func (r *runtime) do(job func()) bool {
	if job == nil {
		return false
	}
	done := make(chan struct{})
	wrapped := func() {
		defer close(done)
		job()
	}
	select {
	case <-r.quit:
		return false
	case r.jobs <- wrapped:
	}
	select {
	case <-done:
		return true
	case <-r.closed:
		return false
	}
}

// context is the parent context for every Lua execution. Cancelling it lets
// stop interrupt an in-flight Lua loop without touching its LState from the
// shutdown goroutine.
func (r *runtime) context() context.Context { return r.ctx }

// stop signals the worker to exit, cancels any in-flight Lua execution, and
// waits for the worker. LStates can be closed safely after it returns.
func (r *runtime) stop() {
	r.closeOnce.Do(func() {
		r.submitMu.Lock()
		close(r.quit)
		r.cancel()
		r.submitMu.Unlock()
	})
	r.wg.Wait()
}
