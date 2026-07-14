package discord

import (
	"log/slog"
	"net/http"
	"time"
)

type discordTransport struct {
	base       http.RoundTripper
	superProps string
}

func newTransport() *discordTransport {
	sp, err := superProperties()
	if err != nil {
		slog.Error("failed to build super-properties", "err", err)
	}
	return &discordTransport{
		base:       http.DefaultTransport,
		superProps: sp,
	}
}

func (t *discordTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())

	// setDefault only fills in a header the caller hasn't already set, so
	// e.g. the remote-auth ticket exchange can supply its own Referer.
	setDefault := func(key, value string) {
		if r.Header.Get(key) == "" {
			r.Header.Set(key, value)
		}
	}

	setDefault("Accept", "*/*")
	setDefault("Accept-Language", "en-US,en;q=0.9")
	setDefault("Content-Type", "application/json")
	setDefault("User-Agent", clientBrowserUA)
	setDefault("Origin", "https://discord.com")
	setDefault("Priority", "u=1, i")
	setDefault("Referer", "https://discord.com/channels/@me")
	setDefault("Sec-Ch-Ua", clientSecCHUA)
	setDefault("Sec-Ch-Ua-Mobile", "?0")
	setDefault("Sec-Ch-Ua-Platform", `"Windows"`)
	setDefault("Sec-Fetch-Dest", "empty")
	setDefault("Sec-Fetch-Mode", "cors")
	setDefault("Sec-Fetch-Site", "same-origin")
	setDefault("X-Debug-Options", "bugReporterEnabled")
	setDefault("X-Discord-Locale", string(clientLocale))
	setDefault("X-Discord-Timezone", time.Now().Location().String())

	if t.superProps != "" {
		setDefault("X-Super-Properties", t.superProps)
	}

	return t.base.RoundTrip(r)
}
