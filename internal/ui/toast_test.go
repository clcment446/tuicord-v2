package ui

import (
	"testing"
	"time"
)

func TestToastExpiry(t *testing.T) {
	now := time.Now()

	timed := NewToast("t", "d", Styles{}).SetTTL(5 * time.Second)
	if timed.expired(now) {
		t.Fatal("toast expired before its TTL")
	}
	if !timed.expired(now.Add(6 * time.Second)) {
		t.Fatal("toast did not expire after its TTL")
	}

	// Expanding (reading) the toast cancels auto-dismiss.
	timed.Toggle()
	if timed.expired(now.Add(time.Hour)) {
		t.Fatal("expanded toast auto-dismissed while being read")
	}

	// A toast with no TTL never auto-expires.
	plain := NewToast("t", "d", Styles{})
	if plain.expired(now.Add(time.Hour)) {
		t.Fatal("no-TTL toast auto-dismissed")
	}

	// A nil toast is safe and never expired.
	var none *Toast
	if none.expired(now) {
		t.Fatal("nil toast reported expired")
	}
}
