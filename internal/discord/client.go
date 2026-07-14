package discord

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/handler"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/google/uuid"
)

const (
	clientOS           = "Windows"
	clientOSVersion    = "10"
	clientBrowser      = "Chrome"
	clientBrowserVer   = "150.0.0.0"
	clientBrowserUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	clientSecCHUA      = `"Not_A Brand";v="99", "Google Chrome";v="150", "Chromium";v="150"`
	clientLocale       = discord.EnglishUS
	clientCapabilities = 16381
)

// BrowserUserAgent returns the browser identity shared by REST and remote-auth
// websocket requests. Keeping one source prevents Discord from seeing two
// different client versions during a single login flow.
func BrowserUserAgent() string { return clientBrowserUA }

// errNoBuildNumber reports that the client build number was not found in the
// fetched Discord app page.
var errNoBuildNumber = errors.New("discord: build number not found in app page")

// NewSession creates an arikawa Session configured like the old tuicord client.
func NewSession(token string) (*session.Session, error) {
	apiCl := newAPIClient(token)

	cmd := gateway.DefaultIdentifyCommand(token)
	cmd.Properties = identifyProperties()
	cmd.Capabilities = clientCapabilities
	cmd.ClientState = &gateway.ClientState{
		GuildHashes:              map[discord.GuildID]interface{}{},
		ReadStateVersion:         0,
		UserGuildSettingsVersion: -1,
		UserSettingsVersion:      -1,
	}
	id := gateway.NewIdentifier(cmd)

	// Discord's current /gateway REST endpoint rejects user-token requests
	// with "Only bots are allowed to use this endpoint". User sessions can
	// connect directly to the public gateway websocket instead.
	gw := gateway.NewCustomWithIdentifier(gateway.AddGatewayParams("wss://gateway.discord.gg/"), id, nil)
	sess := session.NewWithGateway(gw, handler.New())
	// NewWithGateway creates a default REST client; retain the browser-shaped
	// client used for the rest of tuicord's authenticated requests.
	sess.Client = apiCl
	return sess, nil
}

// NewUnauthenticatedClient creates an arikawa API client with the same
// browser-mimicking headers (X-Super-Properties, Sec-Fetch-*, build number,
// etc.) as an authenticated session, but without a token. Discord's fraud
// detection expects these on pre-login requests too, such as the remote-auth
// ticket exchange used by QR login; without them it tends to respond with a
// captcha challenge.
func NewUnauthenticatedClient() *api.Client {
	return newAPIClient("")
}

func newAPIClient(token string) *api.Client {
	httpCl := httputil.NewClient()
	httpCl.Client = httpdriver.WrapClient(http.Client{Transport: newTransport()})

	apiCl := api.NewCustomClient(token, httpCl)
	apiCl.UserAgent = clientBrowserUA
	return apiCl
}

func identifyProperties() gateway.IdentifyProperties {
	return gateway.IdentifyProperties{
		gateway.IdentifyDevice: "",

		gateway.IdentifyOS: clientOS,
		"os_version":       clientOSVersion,

		gateway.IdentifyBrowser: clientBrowser,
		"browser_version":       clientBrowserVer,
		"browser_user_agent":    clientBrowserUA,

		"client_build_number":         clientBuildNumber(),
		"client_event_source":         nil,
		"client_app_state":            "focused",
		"client_launch_id":            uuid.NewString(),
		"client_heartbeat_session_id": uuid.NewString(),

		"launch_signature": generateLaunchSignature(),
		"system_locale":    clientLocale,
		"release_channel":  "stable",
		"has_client_mods":  false,

		"referrer":                 "",
		"referrer_current":         "",
		"referring_domain":         "",
		"referring_domain_current": "",

		"is_fast_connect":         false,
		"gateway_connect_reasons": "AppSkeleton",
	}
}

func superProperties() (string, error) {
	props := identifyProperties()
	delete(props, "is_fast_connect")
	delete(props, "gateway_connect_reasons")

	raw, err := json.Marshal(props)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func generateLaunchSignature() string {
	mask := [16]byte{
		0b11111111, 0b01111111, 0b11101111, 0b11101111,
		0b11110111, 0b11101111, 0b11110111, 0b11111111,
		0b11011111, 0b01111110, 0b11111111, 0b10111111,
		0b11111110, 0b11111111, 0b11110111, 0b11111111,
	}
	id := uuid.New()
	for i := range mask {
		id[i] &= mask[i]
	}
	return id.String()
}
