package discord

import "testing"

func TestNewSessionDoesNotPanic(t *testing.T) {
	sess, err := NewSession("testtoken")
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
}

func TestSuperProperties(t *testing.T) {
	got, err := superProperties()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("super properties is empty")
	}
}
