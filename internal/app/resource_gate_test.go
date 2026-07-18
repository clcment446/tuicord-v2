package app

import "testing"

func TestLoadGateTracksPendingLoadedAndRetryStates(t *testing.T) {
	var gate loadGate[int]

	if !gate.begin(7) {
		t.Fatal("first load should begin")
	}
	if gate.begin(7) {
		t.Fatal("pending load should be suppressed")
	}

	gate.finish(7, false)
	if !gate.begin(7) {
		t.Fatal("failed load should be retryable")
	}

	gate.finish(7, true)
	if gate.begin(7) {
		t.Fatal("successful load should be cached")
	}
}

func TestLoadGateTracksPaginationExhaustion(t *testing.T) {
	var gate loadGate[int]

	if !gate.beginOlder(3) {
		t.Fatal("first page should begin")
	}
	gate.finishOlder(3, false)
	if !gate.beginOlder(3) {
		t.Fatal("non-exhausted page should be retryable")
	}
	gate.finishOlder(3, true)
	if gate.beginOlder(3) {
		t.Fatal("exhausted pagination should be suppressed")
	}
}

func TestLoadGateInvalidationRejectsStaleCompletion(t *testing.T) {
	var gate loadGate[int]
	oldVersion, ok := gate.beginVersion(7)
	if !ok {
		t.Fatal("old lifetime did not begin")
	}
	gate.invalidate(7)
	newVersion, ok := gate.beginVersion(7)
	if !ok || newVersion == oldVersion {
		t.Fatalf("new lifetime begin = %t version %d, old %d", ok, newVersion, oldVersion)
	}
	if gate.finishVersion(7, oldVersion, true) {
		t.Fatal("stale completion was accepted")
	}
	if _, pending := gate.pending[7]; !pending {
		t.Fatal("stale completion cleared new lifetime pending state")
	}
	if !gate.finishVersion(7, newVersion, true) {
		t.Fatal("new lifetime completion was rejected")
	}
	if gate.begin(7) {
		t.Fatal("successful new lifetime was not cached")
	}
}

func TestSingleLoadGateRetriesFailures(t *testing.T) {
	var gate singleLoadGate

	if !gate.begin() || gate.begin() {
		t.Fatal("single load should only begin once while pending")
	}
	gate.finish(false)
	if !gate.begin() {
		t.Fatal("failed single load should be retryable")
	}
	gate.finish(true)
	if gate.begin() {
		t.Fatal("successful single load should be cached")
	}
}
