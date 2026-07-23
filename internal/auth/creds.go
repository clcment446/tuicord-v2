package auth

import (
	"encoding/json"
	"strings"
	"time"
)

// Protocol identifies which chat backend an account speaks. An empty protocol
// means Discord, so registries and keyring blobs written before Matrix support
// keep working unchanged.
const (
	ProtocolDiscord = "discord"
	ProtocolMatrix  = "matrix"
)

// Credentials is the per-account secret payload stored in the OS keyring for a
// Matrix account. Discord accounts continue to store a bare token string, so a
// value that does not decode as JSON with a "protocol" field is treated as a
// legacy Discord token (see Decode).
//
// The whole struct is JSON-encoded under the account's existing keyring key, so
// the access token, device ID, refresh token, and the E2EE pickle key all live
// together and rotate together.
type Credentials struct {
	Protocol string `json:"protocol"`
	// Homeserver is the resolved client-server base URL (post .well-known),
	// e.g. "https://matrix-client.matrix.org".
	Homeserver string `json:"homeserver"`
	UserID     string `json:"user_id"`
	DeviceID   string `json:"device_id"`
	// AccessToken authenticates client-server requests. With OIDC/MAS it is the
	// OAuth access token and rotates via RefreshToken.
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenExpiry  time.Time `json:"token_expiry,omitempty"`
	// PickleKey encrypts the local E2EE crypto store at rest. 32 random bytes,
	// generated once at login and never rotated for the life of the device.
	PickleKey []byte `json:"pickle_key,omitempty"`
	// OIDC fields, set only for accounts logged in through the next-gen auth
	// (MAS) device-authorization flow.
	OIDCIssuer   string `json:"oidc_issuer,omitempty"`
	OIDCClientID string `json:"oidc_client_id,omitempty"`
}

// Encode serializes credentials for keyring storage.
func (c Credentials) Encode() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Decode parses a keyring value. It returns ok=false when the value is not a
// Matrix credentials blob — either an empty string or a legacy bare Discord
// token — so callers can fall back to treating raw as a Discord token.
func Decode(raw string) (c Credentials, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return Credentials{}, false
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Credentials{}, false
	}
	if c.Protocol == "" {
		return Credentials{}, false
	}
	return c, true
}
