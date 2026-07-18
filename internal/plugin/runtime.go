package plugin

import "sync"

// runtime owns the single goroutine on which all Lua executes. Work is a plain
// closure; callers use submit for fire-and-forget dispatch (events, key/command
// handlers) and do for load-time work that must complete before proceeding.
type runtime struct {
	jobs   chan func()
	quit   chan struct{}
	wg     sync.WaitGroup
	closed chan struct{}

	closeOnce sync.Once
}

// newRuntime creates a runtime whose job queue holds queue pending jobs before
// submit starts dropping. start must be called to spin up the goroutine.
func newRuntime(queue int) *runtime {
	if queue < 1 {
		queue = 1
	}
	return &runtime{
		jobs:   make(chan func(), queue),
		quit:   make(chan struct{}),
		closed: make(chan struct{}),
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

// stop signals the worker to exit and waits for the in-flight job to finish.
func (r *runtime) stop() {
	r.closeOnce.Do(func() { close(r.quit) })
	r.wg.Wait()
}
