package plugin

import (
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// timerRegistry owns Go timers while ensuring callbacks always return to the
// manager's single Lua worker. Timers never call an LState directly.
type timerRegistry struct {
	mu      sync.Mutex
	entries []*timerEntry
}

type timerEntry struct {
	owner *lua.LState
	stop  chan struct{}
}

func (r *timerRegistry) every(interval time.Duration, owner *lua.LState, run func()) {
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	e := &timerEntry{owner: owner, stop: make(chan struct{})}
	r.mu.Lock()
	r.entries = append(r.entries, e)
	r.mu.Unlock()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				run()
			case <-e.stop:
				return
			}
		}
	}()
}

func (r *timerRegistry) rollbackOwner(owner *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.entries[:0]
	for _, entry := range r.entries {
		if entry.owner == owner {
			close(entry.stop)
			continue
		}
		kept = append(kept, entry)
	}
	r.entries = kept
}

func (r *timerRegistry) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.entries {
		close(entry.stop)
	}
	r.entries = nil
}
