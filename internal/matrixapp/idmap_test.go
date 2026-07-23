package matrixapp

import (
	"path/filepath"
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
