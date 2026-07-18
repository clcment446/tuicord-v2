package app

// loadGate centralizes the state machine shared by one-shot and paginated
// resource loads. Callers must serialize access when a gate is shared across
// goroutines; App does that with resourceMu.
type loadGate[K comparable] struct {
	loaded    map[K]struct{}
	pending   map[K]struct{}
	exhausted map[K]struct{}
	version   map[K]uint64
}

func (g *loadGate[K]) ensure() {
	if g.loaded == nil {
		g.loaded = make(map[K]struct{})
	}
	if g.pending == nil {
		g.pending = make(map[K]struct{})
	}
	if g.exhausted == nil {
		g.exhausted = make(map[K]struct{})
	}
	if g.version == nil {
		g.version = make(map[K]uint64)
	}
}

func (g *loadGate[K]) begin(key K) bool {
	_, ok := g.beginVersion(key)
	return ok
}

func (g *loadGate[K]) beginVersion(key K) (uint64, bool) {
	g.ensure()
	if _, ok := g.loaded[key]; ok {
		return g.version[key], false
	}
	if _, ok := g.pending[key]; ok {
		return g.version[key], false
	}
	g.pending[key] = struct{}{}
	return g.version[key], true
}

func (g *loadGate[K]) beginOlder(key K) bool {
	_, ok := g.beginOlderVersion(key)
	return ok
}

func (g *loadGate[K]) beginOlderVersion(key K) (uint64, bool) {
	g.ensure()
	if _, ok := g.pending[key]; ok {
		return g.version[key], false
	}
	if _, ok := g.exhausted[key]; ok {
		return g.version[key], false
	}
	g.pending[key] = struct{}{}
	return g.version[key], true
}

func (g *loadGate[K]) finish(key K, ok bool) {
	g.finishVersion(key, g.version[key], ok)
}

func (g *loadGate[K]) finishVersion(key K, version uint64, ok bool) bool {
	g.ensure()
	if g.version[key] != version {
		return false
	}
	delete(g.pending, key)
	if ok {
		g.loaded[key] = struct{}{}
	}
	return true
}

func (g *loadGate[K]) finishOlder(key K, exhausted bool) {
	g.finishOlderVersion(key, g.version[key], exhausted)
}

func (g *loadGate[K]) finishOlderVersion(key K, version uint64, exhausted bool) bool {
	g.ensure()
	if g.version[key] != version {
		return false
	}
	delete(g.pending, key)
	if exhausted {
		g.exhausted[key] = struct{}{}
	}
	return true
}

func (g *loadGate[K]) markLoaded(key K) {
	g.ensure()
	g.version[key]++
	g.loaded[key] = struct{}{}
	delete(g.pending, key)
}

func (g *loadGate[K]) markExhausted(key K) {
	g.ensure()
	g.exhausted[key] = struct{}{}
}

// invalidate forgets every state associated with key. Lifecycle delete
// handlers use it so a resource recreated with the same snowflake can load
// again. Version-aware completions cannot mutate this fresh gate state.
func (g *loadGate[K]) invalidate(key K) {
	g.ensure()
	g.version[key]++
	delete(g.loaded, key)
	delete(g.pending, key)
	delete(g.exhausted, key)
}

type singleLoadGate struct {
	loaded  bool
	pending bool
	version uint64
}

func (g *singleLoadGate) begin() bool {
	_, ok := g.beginVersion()
	return ok
}

func (g *singleLoadGate) beginVersion() (uint64, bool) {
	if g.loaded || g.pending {
		return g.version, false
	}
	g.pending = true
	return g.version, true
}

func (g *singleLoadGate) finish(ok bool) {
	g.finishVersion(g.version, ok)
}

func (g *singleLoadGate) finishVersion(version uint64, ok bool) bool {
	if g.version != version {
		return false
	}
	g.pending = false
	if ok {
		g.loaded = true
	}
	return true
}

func (g *singleLoadGate) invalidate() {
	g.version++
	g.loaded = false
	g.pending = false
}
