package matrixapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// TestEncryptedRoomBlocksPlaintextSend guards the plaintext-leak fix: posting to
// an encrypted room while E2EE is not ready must be refused (not sent as
// plaintext) and surface an actionable error. The guard runs before the client
// is touched, so it holds even with no live transport.
func TestEncryptedRoomBlocksPlaintextSend(t *testing.T) {
	a, st := newTestApp()
	room := id.RoomID("!enc:example.org")
	channel := a.channelFor(room)
	a.onStateEncryption(context.Background(), &event.Event{Type: event.StateEncryption, RoomID: room})

	var reported error
	a.onError = func(err error) { reported = err }

	a.SendToChannel(channel, "secret")

	if got := len(st.Messages(channel)); got != 0 {
		t.Fatalf("blocked send appended %d messages, want 0", got)
	}
	if !errors.Is(reported, errEncryptionUnavailable) {
		t.Fatalf("expected encryption-unavailable error, got %v", reported)
	}
}

// TestBlockedByEncryptionGate covers the three states of the send gate.
func TestBlockedByEncryptionGate(t *testing.T) {
	a, _ := newTestApp()
	room := id.RoomID("!room:example.org")
	channel := a.channelFor(room)

	// Unencrypted room: never blocked.
	if a.blockedByEncryption(channel) {
		t.Fatal("unencrypted room should not be blocked")
	}

	a.onStateEncryption(context.Background(), &event.Event{Type: event.StateEncryption, RoomID: room})

	// Encrypted + crypto not ready: blocked.
	if !a.blockedByEncryption(channel) {
		t.Fatal("encrypted room with crypto down should be blocked")
	}

	// Encrypted + crypto ready: allowed.
	a.cryptoReady.Store(true)
	if a.blockedByEncryption(channel) {
		t.Fatal("encrypted room with crypto ready should not be blocked")
	}
}

// TestCryptoSetupErrorActionable checks the device-conflict message replaces the
// cryptic mautrix string.
func TestCryptoSetupErrorActionable(t *testing.T) {
	err := cryptoSetupError(errors.New("olm account is not marked as shared, but there are keys on the server"))
	if err == nil || !strings.Contains(err.Error(), "device of its own") {
		t.Fatalf("expected actionable device-conflict message, got %v", err)
	}
	if cryptoSetupError(nil) != nil {
		t.Fatal("nil error should map to nil")
	}
}
