package app

import "github.com/google/uuid"

// newNonce returns a unique message nonce used to reconcile an optimistic local
// echo with the gateway's confirmation of the same message.
func newNonce() string {
	return uuid.NewString()
}
