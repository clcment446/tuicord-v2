package app

// loadGate centralizes the state machine shared by one-shot and paginated
// resource loads. Callers must serialize access when a gate is shared across
// goroutines; App does that with resourceMu.
type loadGate[K comparable] struct {
	loaded    map[K]struct{}
	pending   map[K]struct{}
	exhausted map[K]struct{}
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
}

func (g *loadGate[K]) begin(key K) bool {
	g.ensure()
	if _, ok := g.loaded[key]; ok {
		return false
	}
	if _, ok := g.pending[key]; ok {
		return false
	}
	g.pending[key] = struct{}{}
	return true
}

func (g *loadGate[K]) beginOlder(key K) bool {
	g.ensure()
	if _, ok := g.pending[key]; ok {
		return false
	}
	if _, ok := g.exhausted[key]; ok {
		return false
	}
	g.pending[key] = struct{}{}
	return true
}

func (g *loadGate[K]) finish(key K, ok bool) {
	g.ensure()
	delete(g.pending, key)
	if ok {
		g.loaded[key] = struct{}{}
	}
}

func (g *loadGate[K]) finishOlder(key K, exhausted bool) {
	g.ensure()
	delete(g.pending, key)
	if exhausted {
		g.exhausted[key] = struct{}{}
	}
}

func (g *loadGate[K]) markLoaded(key K) {
	g.ensure()
	g.loaded[key] = struct{}{}
	delete(g.pending, key)
}

func (g *loadGate[K]) markExhausted(key K) {
	g.ensure()
	g.exhausted[key] = struct{}{}
}

type singleLoadGate struct {
	loaded  bool
	pending bool
}

func (g *singleLoadGate) begin() bool {
	if g.loaded || g.pending {
		return false
	}
	g.pending = true
	return true
}

func (g *singleLoadGate) finish(ok bool) {
	g.pending = false
	if ok {
		g.loaded = true
	}
}
