package matrixapp

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"awesomeProject/internal/atomicfile"
)

// eventIDBase is the start of the ephemeral event-ID range. Rooms and users are
// interned from 1 upward and persisted; event IDs are allocated from this high,
// non-persisted range so the two spaces never collide even across restarts.
const eventIDBase uint64 = 1 << 48

// eventIDCap bounds how many interned event IDs are retained. It is far larger
// than the total number of messages the store keeps across all channel rings,
// so an evicted event's message is always already gone from every ring — which
// makes eviction safe: the dedup guard (HasMessage) only consults live rings,
// and a redelivered event that old would legitimately be treated as new.
const eventIDCap = 65536

// idMap interns Matrix string IDs (!room:server, @user:server, $event) into the
// store's uint64 identifier space. Rooms and users are persisted so that
// uistate layouts — keyed by uint64 — stay valid across restarts; event IDs are
// ephemeral (the store never persists message IDs) and bounded by a FIFO cap so
// a long-running session does not grow the map without limit.
//
// It is safe for concurrent use: the sync goroutine and action goroutines
// allocate, while the UI goroutine resolves reverse lookups.
type idMap struct {
	mu   sync.RWMutex
	path string

	fwd        map[string]uint64 // mxid -> id (rooms + users + events)
	rev        map[uint64]string // id -> mxid (rooms + users + events)
	next       uint64            // next persistent id
	enext      uint64            // next ephemeral event id
	eventOrder []string          // interned event mxids in allocation order (FIFO)
	dirty      bool
}

type idMapFile struct {
	Next    uint64            `json:"next"`
	Entries map[string]uint64 `json:"entries"`
}

// newIDMap loads a persisted map from path (missing/corrupt file starts empty).
// A zero path disables persistence (used by tests).
func newIDMap(path string) *idMap {
	m := &idMap{
		path:  path,
		fwd:   map[string]uint64{},
		rev:   map[uint64]string{},
		next:  1,
		enext: eventIDBase,
	}
	if path == "" {
		return m
	}
	if raw, err := os.ReadFile(path); err == nil {
		var f idMapFile
		if json.Unmarshal(raw, &f) == nil {
			for mxid, id := range f.Entries {
				m.fwd[mxid] = id
				m.rev[id] = mxid
			}
			if f.Next > m.next {
				m.next = f.Next
			}
		}
	}
	return m
}

// intern maps a persistent mxid (room or user) to a stable uint64, allocating a
// new one the first time it is seen.
func (m *idMap) intern(mxid string) uint64 {
	if mxid == "" {
		return 0
	}
	m.mu.RLock()
	id, ok := m.fwd[mxid]
	m.mu.RUnlock()
	if ok {
		return id
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok = m.fwd[mxid]; ok {
		return id
	}
	id = m.next
	m.next++
	m.fwd[mxid] = id
	m.rev[id] = mxid
	m.dirty = true
	return id
}

// event maps an event ID to an ephemeral uint64. Repeated calls for the same
// event return the same id within a process lifetime.
func (m *idMap) event(mxid string) uint64 {
	if mxid == "" {
		return 0
	}
	m.mu.RLock()
	id, ok := m.fwd[mxid]
	m.mu.RUnlock()
	if ok {
		return id
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok = m.fwd[mxid]; ok {
		return id
	}
	id = m.enext
	m.enext++
	m.fwd[mxid] = id
	m.rev[id] = mxid
	m.eventOrder = append(m.eventOrder, mxid)
	if len(m.eventOrder) > eventIDCap {
		evict := m.eventOrder[0]
		m.eventOrder = m.eventOrder[1:]
		if old, ok := m.fwd[evict]; ok {
			delete(m.fwd, evict)
			delete(m.rev, old)
		}
	}
	return id
}

// str resolves a uint64 back to its Matrix string ID.
func (m *idMap) str(id uint64) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mxid, ok := m.rev[id]
	return mxid, ok
}

// flush persists the room/user table if it changed since the last flush. Event
// IDs are excluded (they live in the ephemeral range at or above eventIDBase).
func (m *idMap) flush() {
	m.mu.Lock()
	if !m.dirty || m.path == "" {
		m.mu.Unlock()
		return
	}
	f := idMapFile{Next: m.next, Entries: make(map[string]uint64, len(m.fwd))}
	for mxid, id := range m.fwd {
		if id < eventIDBase {
			f.Entries[mxid] = id
		}
	}
	m.dirty = false
	m.mu.Unlock()

	_ = atomicfile.Write(m.path, 0o600, func(w io.Writer) error {
		return json.NewEncoder(w).Encode(f)
	})
}
