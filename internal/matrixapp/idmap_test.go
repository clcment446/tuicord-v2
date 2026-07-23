package matrixapp

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestIDMapInternIsStableAndDistinct(t *testing.T) {
	m := newIDMap("")
	room := m.intern("!room:example.org")
	user := m.intern("@alice:example.org")
	if room == 0 || user == 0 || room == user {
		t.Fatalf("intern returned bad ids: room=%d user=%d", room, user)
	}
	if again := m.intern("!room:example.org"); again != room {
		t.Fatalf("intern not stable: %d != %d", again, room)
	}
	if mxid, ok := m.str(room); !ok || mxid != "!room:example.org" {
		t.Fatalf("reverse lookup failed: %q ok=%v", mxid, ok)
	}
}

func TestIDMapEventsUseDisjointRange(t *testing.T) {
	m := newIDMap("")
	persistent := m.intern("!room:example.org")
	ev := m.event("$event123")
	if ev < eventIDBase {
		t.Fatalf("event id %d not in ephemeral range (>= %d)", ev, eventIDBase)
	}
	if persistent >= eventIDBase {
		t.Fatalf("persistent id %d leaked into ephemeral range", persistent)
	}
	if again := m.event("$event123"); again != ev {
		t.Fatalf("event intern not stable: %d != %d", again, ev)
	}
}

func TestIDMapBoundsEventRange(t *testing.T) {
	m := newIDMap("")
	room := m.intern("!room:example.org") // persistent, must never be evicted

	// Intern well past the cap; the event maps must stay bounded.
	total := eventIDCap + 1000
	for i := 0; i < total; i++ {
		m.event("$evt-" + strconv.Itoa(i))
	}
	if got := len(m.eventOrder); got > eventIDCap {
		t.Fatalf("eventOrder = %d, want <= cap %d", got, eventIDCap)
	}
	// The persistent room id survives regardless of event churn.
	if _, ok := m.str(room); !ok {
		t.Fatalf("persistent room id was evicted")
	}
	// The most recent event is still resolvable; an early one is gone.
	if _, ok := m.fwd["$evt-"+strconv.Itoa(total-1)]; !ok {
		t.Fatalf("most recent event id was evicted")
	}
	if _, ok := m.fwd["$evt-0"]; ok {
		t.Fatalf("oldest event id should have been evicted")
	}
	// fwd/rev must not exceed the cap plus the handful of persistent entries.
	if len(m.fwd) > eventIDCap+8 || len(m.rev) > eventIDCap+8 {
		t.Fatalf("maps unbounded: fwd=%d rev=%d", len(m.fwd), len(m.rev))
	}
}

func TestIDMapPersistsRoomsNotEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idmap.json")
	m := newIDMap(path)
	room := m.intern("!room:example.org")
	_ = m.event("$ephemeral")
	m.flush()

	reloaded := newIDMap(path)
	if got := reloaded.intern("!room:example.org"); got != room {
		t.Fatalf("room id not persisted: got %d want %d", got, room)
	}
	// The ephemeral event must not have been persisted.
	if _, ok := reloaded.str(m.fwd["$ephemeral"]); ok {
		t.Fatalf("ephemeral event id was persisted")
	}
}
