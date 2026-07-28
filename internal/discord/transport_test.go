package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awesomeProject/internal/telemetry"
)

func TestTransportAddsCoherentBrowserClientInfo(t *testing.T) {
	withCachedBuildNumber(t, 123456)

	capture := &headerCaptureTransport{}
	transport := newTransport()
	transport.base = capture

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://discord.com/api/v9/users/@me/remote-auth/login", strings.NewReader(`{"ticket":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	h := capture.header
	for key, want := range map[string]string{
		"User-Agent":         clientBrowserUA,
		"Sec-Ch-Ua":          clientSecCHUA,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Origin":             "https://discord.com",
		"Content-Type":       "application/json",
		"X-Discord-Locale":   string(clientLocale),
	} {
		if got := h.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	raw, err := base64.StdEncoding.DecodeString(h.Get("X-Super-Properties"))
	if err != nil {
		t.Fatal(err)
	}
	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		t.Fatal(err)
	}
	if got := props["browser_user_agent"]; got != clientBrowserUA {
		t.Errorf("super-properties browser_user_agent = %v", got)
	}
	if got := props["browser_version"]; got != clientBrowserVer {
		t.Errorf("super-properties browser_version = %v, want %q", got, clientBrowserVer)
	}
	if got := props["browser"]; got != clientBrowser {
		t.Errorf("super-properties browser = %v, want %q", got, clientBrowser)
	}
}

func TestTransportRecordsRedactedRequestResponseAndPermissions(t *testing.T) {
	recorder, err := telemetry.New(filepath.Join(t.TempDir(), "telemetry"), "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	recorder.SetPermissionResolver(func(ids []uint64) []telemetry.PermissionSnapshot {
		return []telemetry.PermissionSnapshot{{ChannelID: ids[0], Known: true, CanRead: true}}
	})
	transport := newTransportWithTelemetry(recorder)
	transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"X-Debug": {"visible"}, "Set-Cookie": {"secret"}}, Body: io.NopCloser(strings.NewReader(`{"error":"denied"}`)), Request: req}, nil
	})
	req, err := http.NewRequest(http.MethodPost, "https://discord.com/api/v9/channels/123/messages", strings.NewReader(`{"token":"secret-token","content":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "secret-token")
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if string(body) != `{"error":"denied"}` {
		t.Fatalf("response body was not preserved: %s", body)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(recorder.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-token") || strings.Contains(string(data), "Set-Cookie") && strings.Contains(string(data), "secret") {
		t.Fatalf("telemetry leaked secret: %s", data)
	}
	var event telemetry.Event
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var candidate telemetry.Event
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatal(err)
		}
		if candidate.Kind == "rest_request" {
			event = candidate
			break
		}
	}
	if event.Endpoint != "/api/v9/channels/:id/messages" || event.Status != http.StatusForbidden || len(event.Permissions) != 1 || event.Permissions[0].ChannelID != 123 {
		t.Fatalf("event = %+v", event)
	}
	if event.ResponseHeaders["X-Debug"] != "visible" || event.ResponseHeaders["Set-Cookie"] != "<redacted>" {
		t.Fatalf("response headers = %#v", event.ResponseHeaders)
	}
}

type headerCaptureTransport struct {
	header http.Header
}

func (t *headerCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.header = req.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}
