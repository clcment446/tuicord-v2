package app

import (
	"strings"

	"github.com/google/uuid"
)

// newNonce returns a unique message nonce (≤25 chars) used to reconcile an
// optimistic local echo with the gateway's confirmation of the same message.
// Discord rejects nonces longer than 25 characters (NONCE_TYPE_TOO_LONG).
func newNonce() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:25]
}
