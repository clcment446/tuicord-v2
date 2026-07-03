package discord

import (
	"context"

	"github.com/diamondburned/arikawa/v3/session"
)

// Connect opens the gateway connection and blocks until ctx is cancelled.
// Intents are not set; this mirrors tuicord's user-token identify behavior.
func Connect(ctx context.Context, s *session.Session) error {
	return s.Connect(ctx)
}
