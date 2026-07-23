package matrixapp

import (
	"net/http"
	"strings"
	"sync"

	"maunium.net/go/mautrix/event"

	"awesomeProject/internal/matrix"
	"awesomeProject/internal/media"
)

// mediaAuthorizer adds the account's bearer token to authenticated-media
// requests aimed at its homeserver and decrypts encrypted attachments. It
// implements media.FetchAuthorizer and is registered process-wide; it only
// claims URLs on its own homeserver, so multiple accounts (and Discord) coexist.
type mediaAuthorizer struct {
	app    *App
	prefix string // homeserver base + "/_matrix/client/v1/media/"

	mu        sync.RWMutex
	encrypted map[string]*event.EncryptedFileInfo // download URL -> keys
}

func newMediaAuthorizer(a *App) *mediaAuthorizer {
	m := &mediaAuthorizer{
		app:       a,
		prefix:    strings.TrimSuffix(a.client.Creds.Homeserver, "/") + "/_matrix/client/v1/media/",
		encrypted: map[string]*event.EncryptedFileInfo{},
	}
	media.RegisterAuthorizer(m)
	return m
}

// registerEncrypted records the decryption keys for an encrypted attachment URL.
func (m *mediaAuthorizer) registerEncrypted(url string, file *event.EncryptedFileInfo) {
	if url == "" || file == nil {
		return
	}
	m.mu.Lock()
	m.encrypted[url] = file
	m.mu.Unlock()
}

// Authorize implements media.FetchAuthorizer.
func (m *mediaAuthorizer) Authorize(url string) (http.Header, func([]byte) ([]byte, error), bool) {
	if !strings.HasPrefix(url, m.prefix) {
		return nil, nil, false
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+m.app.client.Creds.AccessToken)

	m.mu.RLock()
	file := m.encrypted[url]
	m.mu.RUnlock()
	if file == nil {
		return header, nil, true
	}
	transform := func(raw []byte) ([]byte, error) {
		return matrix.DecryptAttachment(file, raw)
	}
	return header, transform, true
}
