package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBrowserExchangeResultDecodesJSONString(t *testing.T) {
	encoded := `{"status":200,"body":"{\"encrypted_token\":\"ciphertext\"}"}`
	var result struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != 200 || result.Body != `{"encrypted_token":"ciphertext"}` {
		t.Fatalf("result = %+v", result)
	}
}

func TestLaunchFirefoxRemovesOwnedProfileWhenStartFails(t *testing.T) {
	tempRoot := t.TempDir()
	setTempRoot(t, tempRoot)
	missingBinary := filepath.Join(tempRoot, "missing-firefox")

	if _, err := LaunchFirefox(context.Background(), FirefoxOptions{Binary: missingBinary}); err == nil {
		t.Fatal("LaunchFirefox succeeded with a missing binary")
	}
	assertDirectoryEmpty(t, tempRoot)
}

func TestLaunchFirefoxRemovesOwnedProfileAfterProcessStartFailure(t *testing.T) {
	tempRoot := t.TempDir()
	setTempRoot(t, tempRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if _, err := LaunchFirefox(ctx, FirefoxOptions{Binary: os.Args[0], Headless: true}); err == nil {
		t.Fatal("LaunchFirefox unexpectedly connected to test process")
	}
	assertDirectoryEmpty(t, tempRoot)
}

func TestLaunchFirefoxNeverRemovesCallerProfileOnFailure(t *testing.T) {
	tempRoot := t.TempDir()
	profile := filepath.Join(tempRoot, "caller-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(profile, "keep")
	if err := os.WriteFile(marker, []byte("caller-owned"), 0o600); err != nil {
		t.Fatal(err)
	}

	missingBinary := filepath.Join(tempRoot, "missing-firefox")
	if _, err := LaunchFirefox(context.Background(), FirefoxOptions{Binary: missingBinary, Profile: profile}); err == nil {
		t.Fatal("LaunchFirefox succeeded with a missing binary")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("caller profile was removed: %v", err)
	}
}

func TestSessionCloseWaitsThenRemovesOwnedProfileAndIsIdempotent(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "owned-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFirefoxProcessHelper$")
	cmd.Env = append(os.Environ(), "TUICORD_FIREFOX_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &Session{cmd: cmd, profile: profile, ownsProfile: true}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("Close returned before the Firefox process was reaped")
	}
	if _, err := os.Stat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned profile still exists after Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSessionCloseNeverRemovesCallerProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "caller-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Session{profile: profile}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("caller profile was removed: %v", err)
	}
}

func TestCommandHonorsCallerContextWhileResponseIsBlocked(t *testing.T) {
	s, received := newBlockedBiDiSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	started := time.Now()
	err := s.command(ctx, "test.blocked", map[string]any{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("command returned after %v; caller context was not prompt", elapsed)
	}
	select {
	case <-received:
	default:
		t.Fatal("server did not receive command")
	}
}

func TestSessionCloseInterruptsBlockedCommand(t *testing.T) {
	s, received := newBlockedBiDiSession(t)
	commandDone := make(chan error, 1)
	go func() {
		commandDone <- s.command(context.Background(), "test.blocked", map[string]any{}, nil)
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("server did not receive command")
	}

	started := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Close returned after %v; blocked read was not interrupted", elapsed)
	}
	select {
	case err := <-commandDone:
		if err == nil {
			t.Fatal("blocked command succeeded after Close")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked command did not return after Close")
	}
}

func newBlockedBiDiSession(t *testing.T) (*Session, <-chan struct{}) {
	t.Helper()
	received := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request map[string]any
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		close(received)
		// Wait for the client to close instead of replying.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	s := &Session{conn: conn}
	t.Cleanup(func() {
		_ = s.Close()
		server.Close()
	})
	return s, received
}

func TestFirefoxProcessHelper(t *testing.T) {
	if os.Getenv("TUICORD_FIREFOX_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func setTempRoot(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
}

func assertDirectoryEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary Firefox profile leaked: %v", entries)
	}
}
