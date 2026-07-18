package captcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// FirefoxOptions configures an isolated Firefox instance controlled through
// WebDriver BiDi. Firefox 141+ exposes BiDi as its supported remote protocol.
type FirefoxOptions struct {
	Binary   string
	Profile  string
	URL      string
	Width    int
	Height   int
	Headless bool
}

// Challenge contains the challenge metadata returned by Discord. The values
// are session-bound and must never be logged or persisted.
type Challenge struct {
	SiteKey      string
	RequestData  string
	RequestToken string
	SessionID    string
}

// Session is a small BiDi client for the browser operations needed by the
// CAPTCHA surface. It intentionally exposes only screenshots and genuine user
// input actions; it has no synthetic-motion or CAPTCHA-solving helpers.
type Session struct {
	conn        *websocket.Conn
	cmd         *exec.Cmd
	profile     string
	ownsProfile bool
	mu          sync.Mutex
	next        int
	ctx         string
}

type bidiMessage struct {
	ID      int             `json:"id,omitempty"`
	Type    string          `json:"type,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// LaunchFirefox starts a dedicated Firefox profile and opens URL.
func LaunchFirefox(ctx context.Context, opts FirefoxOptions) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	binary := opts.Binary
	if binary == "" {
		binary = "firefox"
	}
	if opts.URL == "" {
		opts.URL = "https://discord.com/login"
	}
	if opts.Width <= 0 {
		opts.Width = 1280
	}
	if opts.Height <= 0 {
		opts.Height = 720
	}
	profile := opts.Profile
	ownsProfile := false
	if profile == "" {
		dir, err := os.MkdirTemp("", "tuicord-firefox-")
		if err != nil {
			return nil, fmt.Errorf("create Firefox profile: %w", err)
		}
		profile = dir
		ownsProfile = true
	}
	s := &Session{profile: profile, ownsProfile: ownsProfile}
	complete := false
	defer func() {
		if !complete {
			_ = s.Close()
		}
	}()
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, fmt.Errorf("create Firefox profile: %w", err)
	}

	port, err := freePort()
	if err != nil {
		return nil, err
	}
	args := []string{
		"--profile", filepath.Clean(profile),
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--width", strconv.Itoa(opts.Width),
		"--height", strconv.Itoa(opts.Height),
		opts.URL,
	}
	if opts.Headless {
		args = append([]string{"--headless"}, args...)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	s.cmd = cmd
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Firefox: %w", err)
	}

	endpoint := fmt.Sprintf("ws://127.0.0.1:%d/session", port)
	var conn *websocket.Conn
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect to Firefox BiDi: %w", err)
	}

	s.conn = conn
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	complete = true
	return s, nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find Firefox debug port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func (s *Session) init(ctx context.Context) error {
	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := s.command(ctx, "session.new", map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{}}}, &result); err != nil {
		return fmt.Errorf("initialize Firefox BiDi: %w", err)
	}
	var tree struct {
		Contexts []struct {
			ID string `json:"context"`
		} `json:"contexts"`
	}
	if err := s.command(ctx, "browsingContext.getTree", map[string]any{}, &tree); err != nil {
		return fmt.Errorf("get Firefox browsing context: %w", err)
	}
	if len(tree.Contexts) == 0 || tree.Contexts[0].ID == "" {
		return errors.New("Firefox returned no browsing context")
	}
	s.ctx = tree.Contexts[0].ID
	return nil
}

// Screenshot captures the current browser viewport.
func (s *Session) Screenshot(ctx context.Context) (image.Image, error) {
	var result struct {
		Data string `json:"data"`
	}
	if err := s.command(ctx, "browsingContext.captureScreenshot", map[string]any{"context": s.ctx, "origin": "viewport"}, &result); err != nil {
		return nil, fmt.Errorf("capture Firefox screenshot: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decode Firefox screenshot: %w", err)
	}
	img, _, err := image.Decode(bytesReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode Firefox screenshot image: %w", err)
	}
	return img, nil
}

// PerformActions sends genuine browser input actions. Coordinates are browser
// viewport pixels calculated from the user's terminal event.
func (s *Session) PerformActions(ctx context.Context, actions []map[string]any) error {
	return s.command(ctx, "input.performActions", map[string]any{
		"context": s.ctx,
		"actions": actions,
	}, nil)
}

// Evaluate executes JavaScript in the current browser context and decodes its
// JSON-serializable return value. It is used only for rendering the official
// hCaptcha widget and exchanging the resulting browser response.
func (s *Session) Evaluate(ctx context.Context, expression string, result any) error {
	var response struct {
		Result struct {
			Type      string          `json:"type"`
			Value     json.RawMessage `json:"value"`
			Exception string          `json:"exceptionDetails,omitempty"`
		} `json:"result"`
	}
	if err := s.command(ctx, "script.evaluate", map[string]any{
		"expression":      expression,
		"target":          map[string]any{"context": s.ctx},
		"awaitPromise":    true,
		"resultOwnership": "none",
	}, &response); err != nil {
		return fmt.Errorf("evaluate Firefox script: %w", err)
	}
	if response.Result.Type == "exception" {
		return errors.New("Firefox script raised an exception")
	}
	if result == nil || len(response.Result.Value) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Result.Value, result); err != nil {
		return fmt.Errorf("decode Firefox script result: %w", err)
	}
	return nil
}

// RenderChallenge mounts the official hCaptcha widget in the Firefox page.
func (s *Session) RenderChallenge(ctx context.Context, challenge Challenge) error {
	siteKey, _ := json.Marshal(challenge.SiteKey)
	rqData, _ := json.Marshal(challenge.RequestData)
	expression := fmt.Sprintf(`(() => {
const sitekey=%s, rqdata=%s;
window.__tuicordCaptchaResponse = null;
let host=document.getElementById("tuicord-captcha");
if (!host) { host=document.createElement("div"); host.id="tuicord-captcha"; Object.assign(host.style,{position:"fixed",inset:"0",zIndex:"2147483647",background:"rgba(0,0,0,.72)",display:"grid",placeItems:"center"}); document.body.appendChild(host); }
const mount=()=>{ if (window.hcaptcha && !host.dataset.rendered) { window.hcaptcha.render(host,{sitekey,rqdata,callback:(token)=>{window.__tuicordCaptchaResponse=token;}}); host.dataset.rendered="1"; } };
if (window.hcaptcha) mount(); else { const script=document.createElement("script"); script.src="https://js.hcaptcha.com/1/api.js?render=explicit"; script.onload=mount; document.head.appendChild(script); }
return true;
})()`, string(siteKey), string(rqData))
	return s.Evaluate(ctx, expression, nil)
}

// CaptchaResponse returns the user-completed hCaptcha response, if available.
func (s *Session) CaptchaResponse(ctx context.Context) (string, error) {
	var token *string
	if err := s.Evaluate(ctx, "window.__tuicordCaptchaResponse || null", &token); err != nil {
		return "", err
	}
	if token == nil {
		return "", nil
	}
	return *token, nil
}

// ExchangeRemoteAuth submits the ticket through the browser's authenticated
// origin after the user completes the CAPTCHA.
func (s *Session) ExchangeRemoteAuth(ctx context.Context, ticket, fingerprint string, challenge Challenge, captchaKey string) (string, error) {
	ticketJSON, _ := json.Marshal(ticket)
	fingerprintJSON, _ := json.Marshal(fingerprint)
	rqTokenJSON, _ := json.Marshal(challenge.RequestToken)
	sessionJSON, _ := json.Marshal(challenge.SessionID)
	keyJSON, _ := json.Marshal(captchaKey)
	// Return a JSON string rather than a JavaScript object. BiDi represents
	// remote objects as arrays of key/value entries, which cannot be unmarshaled
	// directly into a Go struct.
	expression := fmt.Sprintf(`fetch("/api/v9/users/@me/remote-auth/login",{method:"POST",credentials:"include",headers:{"Content-Type":"application/json","X-Fingerprint":%s,"X-Captcha-Rqtoken":%s,"X-Captcha-Session-Id":%s},body:JSON.stringify({ticket:%s,captcha_key:%s})}).then(async r=>JSON.stringify({status:r.status,body:await r.text()}))`, string(fingerprintJSON), string(rqTokenJSON), string(sessionJSON), string(ticketJSON), string(keyJSON))
	var encodedResponse *string
	if err := s.Evaluate(ctx, expression, &encodedResponse); err != nil {
		return "", err
	}
	if encodedResponse == nil {
		return "", errors.New("browser exchange returned no response")
	}
	var response struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(*encodedResponse), &response); err != nil {
		return "", fmt.Errorf("decode browser exchange result: %w", err)
	}
	if response.Status < 200 || response.Status > 299 {
		return "", fmt.Errorf("Discord browser exchange returned status %d", response.Status)
	}
	var body struct {
		EncryptedToken string `json:"encrypted_token"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		return "", fmt.Errorf("decode browser exchange response: %w", err)
	}
	if body.EncryptedToken == "" {
		return "", errors.New("browser exchange returned no encrypted token")
	}
	return body.EncryptedToken, nil
}

// Close ends the BiDi session and Firefox process. Profiles created by
// LaunchFirefox are removed only after the process has terminated; a profile
// supplied by the caller is never removed. Close is safe to call repeatedly.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.WriteJSON(map[string]any{"id": s.next + 1, "method": "session.end", "params": map[string]any{}})
		_ = s.conn.Close()
		s.conn = nil
	}
	if s.cmd != nil {
		cmd := s.cmd
		s.cmd = nil
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			// Wait releases the process handle before profile removal, which is
			// required on Windows and prevents zombies on Unix.
			_ = cmd.Wait()
		}
	}
	if !s.ownsProfile {
		s.profile = ""
		return nil
	}
	if err := os.RemoveAll(s.profile); err != nil {
		return fmt.Errorf("remove temporary Firefox profile: %w", err)
	}
	s.profile = ""
	s.ownsProfile = false
	return nil
}

func (s *Session) command(ctx context.Context, method string, params map[string]any, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return errors.New("Firefox BiDi session is closed")
	}
	s.next++
	id := s.next
	if err := s.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			return err
		}
		var msg bidiMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.ID != id {
			continue
		}
		if msg.Error != "" {
			if msg.Message != "" {
				return fmt.Errorf("%s: %s", msg.Error, msg.Message)
			}
			return errors.New(msg.Error)
		}
		if result == nil || len(msg.Result) == 0 {
			return nil
		}
		return json.Unmarshal(msg.Result, result)
	}
}

// bytesReader avoids exposing the screenshot buffer beyond image.Decode.
func bytesReader(data []byte) io.Reader { return &byteReader{data: data} }

type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
