package discord

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

	"awesomeProject/internal/telemetry"
)

type discordTransport struct {
	base       http.RoundTripper
	superProps string
	recorder   *telemetry.Recorder
}

func newTransport() *discordTransport {
	return newTransportWithTelemetry(nil)
}

func newTransportWithTelemetry(recorder *telemetry.Recorder) *discordTransport {
	sp, err := superProperties()
	if err != nil {
		slog.Error("failed to build super-properties", "err", err)
	}
	return &discordTransport{
		base:       http.DefaultTransport,
		superProps: sp,
		recorder:   recorder,
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

	var requestBody []byte
	if r.Body != nil {
		requestBody, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(requestBody))
		r.ContentLength = int64(len(requestBody))
	}
	started := time.Now()
	resp, err := t.base.RoundTrip(r)
	if t.recorder == nil {
		return resp, err
	}
	endpoint := telemetry.Endpoint(r.URL)
	e := telemetry.Event{
		Kind: "rest_request", Method: r.Method, Endpoint: endpoint,
		Reason: telemetry.Reason(endpoint), Channels: telemetry.ChannelIDs(r.URL),
		Headers: telemetry.RedactHeaders(r.Header), LatencyMS: time.Since(started).Milliseconds(),
	}
	e.Body, e.BodyBytes = telemetry.RedactBody(requestBody)
	e.Permissions = t.recorder.Permissions(e.Channels)
	if err != nil {
		e.Error = err.Error()
		t.recorder.Record(e)
		return resp, err
	}
	if resp == nil {
		e.Error = "nil response"
		t.recorder.Record(e)
		return resp, nil
	}
	e.Status = resp.StatusCode
	e.ResponseHeaders = telemetry.RedactHeaders(resp.Header)
	for key, values := range resp.Header {
		if len(values) > 0 {
			if e.Fields == nil {
				e.Fields = map[string]any{}
			}
			if key == "Retry-After" || key == "X-RateLimit-Remaining" || key == "X-RateLimit-Reset" || key == "X-RateLimit-Bucket" {
				e.Fields["response_"+key] = values[0]
			}
		}
	}
	resp.Body = &recordingBody{ReadCloser: resp.Body, recorder: t.recorder, event: e, started: started}
	return resp, nil
}

type recordingBody struct {
	io.ReadCloser
	recorder *telemetry.Recorder
	event    telemetry.Event
	started  time.Time
	data     bytes.Buffer
	recorded bool
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.data.Len() < 64<<10 {
		_, _ = b.data.Write(p[:min(n, 64<<10-b.data.Len())])
	}
	if err == io.EOF {
		b.record()
	}
	return n, err
}
func (b *recordingBody) Close() error { err := b.ReadCloser.Close(); b.record(); return err }
func (b *recordingBody) record() {
	if b.recorded {
		return
	}
	b.recorded = true
	b.event.LatencyMS = time.Since(b.started).Milliseconds()
	b.event.Body, b.event.BodyBytes = telemetry.RedactBody(b.data.Bytes())
	b.recorder.Record(b.event)
}
