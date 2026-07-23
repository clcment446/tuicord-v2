// Package matrix is the Matrix transport layer: it builds and authenticates a
// mautrix client with end-to-end encryption wired in, mirroring the role
// internal/discord plays for Discord. It is the only place, together with
// internal/matrixapp, that imports mautrix; the rest of the client depends on
// the protocol-neutral internal/backend seam.
package matrix

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/id"

	// Registers the "sqlite3-fk-wal" database/sql driver (cgo) used by the
	// mautrix crypto/state stores.
	_ "go.mau.fi/util/dbutil/litestream"

	"awesomeProject/internal/auth"
)

// Client is an authenticated, encryption-enabled Matrix session.
type Client struct {
	M      *mautrix.Client
	Crypto *cryptohelper.CryptoHelper
	Creds  auth.Credentials
	dbPath string
}

// New builds a Matrix client from stored credentials. dataDir is the account's
// private directory; the E2EE crypto/state store lives in dataDir/state.db.
// persist is called whenever the access/refresh token rotates so the new blob
// is written back to the keyring; it may run off the UI goroutine.
func New(creds auth.Credentials, dataDir string, persist func(auth.Credentials) error) (*Client, error) {
	if creds.Homeserver == "" || creds.AccessToken == "" || creds.UserID == "" {
		return nil, fmt.Errorf("matrix: incomplete credentials")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("matrix: create data dir: %w", err)
	}

	m, err := mautrix.NewClient(creds.Homeserver, id.UserID(creds.UserID), creds.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("matrix: new client: %w", err)
	}
	m.DeviceID = id.DeviceID(creds.DeviceID)
	m.Log = zerolog.Nop()

	// Token rotation (OIDC/MAS refresh): persist the new blob back to the keyring
	// whenever mautrix refreshes the access token.
	m.SaveNewToken = func(_ context.Context, refresh, access string, expiry time.Time) error {
		creds.AccessToken = access
		creds.RefreshToken = refresh
		creds.TokenExpiry = expiry
		if persist != nil {
			return persist(creds)
		}
		return nil
	}

	c := &Client{M: m, Creds: creds, dbPath: filepath.Join(dataDir, "state.db")}

	pickle := creds.PickleKey
	if len(pickle) == 0 {
		pickle = newPickleKey()
	}
	crypto, err := cryptohelper.NewCryptoHelper(m, pickle, c.dbPath)
	if err != nil {
		return nil, fmt.Errorf("matrix: crypto init: %w", err)
	}
	if err := crypto.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("matrix: crypto start: %w", err)
	}
	m.Crypto = crypto
	c.Crypto = crypto
	return c, nil
}

// Syncer returns the default syncer for registering event handlers. NewClient
// always installs a *DefaultSyncer, so this assertion is safe.
func (c *Client) Syncer() *mautrix.DefaultSyncer {
	return c.M.Syncer.(*mautrix.DefaultSyncer)
}

// UserID returns the logged-in user's Matrix ID.
func (c *Client) UserID() id.UserID { return c.M.UserID }

// newPickleKey returns 32 cryptographically random bytes for the crypto store.
func newPickleKey() []byte {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return key
}
