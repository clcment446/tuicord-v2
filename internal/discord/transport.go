package discord

import (
	"log/slog"
	"net/http"
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

	r.Header.Set("Accept", "*/*")
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.Header.Set("Origin", "https://discord.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://discord.com/channels/@me")
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("X-Debug-Options", "bugReporterEnabled")
	r.Header.Set("X-Discord-Locale", string(clientLocale))

	if t.superProps != "" {
		r.Header.Set("X-Super-Properties", t.superProps)
	}

	return t.base.RoundTrip(r)
}
