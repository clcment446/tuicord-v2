package matrixapp

import (
	"io"

	"awesomeProject/internal/store"
)

// LoadActiveThreads is a no-op for Matrix v1: threads are surfaced inline via
// their reply relation rather than as separate sidebar channels. A future phase
// can map m.thread roots onto the thread UI here.
func (a *App) LoadActiveThreads(guild store.GuildID) {}

// CreateThreadFromMessage is not supported in Matrix v1.
func (a *App) CreateThreadFromMessage(channel store.ChannelID, message store.MessageID, name string) {
}

// readAll reads an upload reader fully.
func readAll(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return io.ReadAll(r)
}
