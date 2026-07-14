package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTransportAddsCoherentBrowserClientInfo(t *testing.T) {
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
