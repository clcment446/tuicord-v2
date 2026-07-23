package matrix

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix"

	"awesomeProject/internal/auth"
)

// DeviceDisplayName is shown in a user's Matrix device list for sessions this
// client creates.
const DeviceDisplayName = "tuicord"

// Discover resolves a user-entered homeserver name to its client-server base
// URL using .well-known discovery, falling back to the input as a direct URL.
func Discover(ctx context.Context, homeserver string) (string, error) {
	homeserver = strings.TrimSpace(homeserver)
	if homeserver == "" {
		return "", fmt.Errorf("matrix: empty homeserver")
	}
	name := strings.TrimPrefix(strings.TrimPrefix(homeserver, "https://"), "http://")
	name = strings.TrimSuffix(name, "/")
	if wk, err := mautrix.DiscoverClientAPI(ctx, name); err == nil && wk != nil && wk.Homeserver.BaseURL != "" {
		return strings.TrimSuffix(wk.Homeserver.BaseURL, "/"), nil
	}
	// No .well-known: treat the input as the base URL directly.
	if strings.Contains(homeserver, "://") {
		return strings.TrimSuffix(homeserver, "/"), nil
	}
	return "https://" + name, nil
}

// LoginPassword performs m.login.password against baseURL and returns a fully
// populated credentials blob (with a fresh pickle key) ready for keyring
// storage. baseURL must already be the resolved client-server URL.
func LoginPassword(ctx context.Context, baseURL, user, password string) (auth.Credentials, error) {
	m, err := mautrix.NewClient(baseURL, "", "")
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("matrix: new client: %w", err)
	}
	resp, err := m.Login(ctx, &mautrix.ReqLogin{
		Type:                     mautrix.AuthTypePassword,
		Identifier:               mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: user},
		Password:                 password,
		InitialDeviceDisplayName: DeviceDisplayName,
		RefreshToken:             true,
	})
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("matrix: login: %w", err)
	}
	return credsFromLogin(baseURL, resp), nil
}

// LoginToken validates an existing access token via /whoami and returns a
// credentials blob. The token is not rotated (no refresh token available).
func LoginToken(ctx context.Context, baseURL, token string) (auth.Credentials, error) {
	m, err := mautrix.NewClient(baseURL, "", strings.TrimSpace(token))
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("matrix: new client: %w", err)
	}
	who, err := m.Whoami(ctx)
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("matrix: validate token: %w", err)
	}
	return auth.Credentials{
		Protocol:    auth.ProtocolMatrix,
		Homeserver:  baseURL,
		UserID:      who.UserID.String(),
		DeviceID:    who.DeviceID.String(),
		AccessToken: strings.TrimSpace(token),
		PickleKey:   newPickleKey(),
	}, nil
}

func credsFromLogin(baseURL string, resp *mautrix.RespLogin) auth.Credentials {
	base := baseURL
	if resp.WellKnown != nil && resp.WellKnown.Homeserver.BaseURL != "" {
		base = strings.TrimSuffix(resp.WellKnown.Homeserver.BaseURL, "/")
	}
	return auth.Credentials{
		Protocol:     auth.ProtocolMatrix,
		Homeserver:   base,
		UserID:       resp.UserID.String(),
		DeviceID:     resp.DeviceID.String(),
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		PickleKey:    newPickleKey(),
	}
}
