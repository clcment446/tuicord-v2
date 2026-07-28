package telemetry

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorderAddsSessionIDWithoutStoringToken(t *testing.T) {
	token := "super-secret-token"
	r, err := New(t.TempDir(), token)
	if err != nil {
		t.Fatal(err)
	}
	r.Record(Event{Kind: "test", Body: `{"token":"super-secret-token","ok":true}`})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) {
		t.Fatal("telemetry contains the raw token")
	}
	var event Event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.SessionID == "" {
			t.Fatal("event has no sessionID")
		}
	}
	sum := sha256.Sum256([]byte(token))
	wantSuffix := hex.EncodeToString(sum[:])
	if !strings.HasSuffix(event.SessionID, "-"+wantSuffix) {
		t.Fatalf("sessionID = %q, want token hash suffix", event.SessionID)
	}
}

func TestRedactHeadersAndJSONBody(t *testing.T) {
	headers := RedactHeaders(map[string][]string{"Authorization": {"secret"}, "X-Test": {"ok"}})
	if headers["Authorization"] != "<redacted>" || headers["X-Test"] != "ok" {
		t.Fatalf("headers = %#v", headers)
	}
	body, _ := RedactBody([]byte(`{"token":"secret","nested":{"password":"pw"},"content":"hello"}`))
	if strings.Contains(body, "secret") || strings.Contains(body, "pw") || !strings.Contains(body, "hello") {
		t.Fatalf("redacted body = %s", body)
	}
}

func TestExportLatestCreatesSafeZip(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "token")
	if err != nil {
		t.Fatal(err)
	}
	r.Record(Event{Kind: "test"})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "support.zip")
	if err := ExportLatest(dir, destination); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}
	if !seen["manifest.json"] || !seen["events.jsonl"] {
		t.Fatalf("zip entries = %#v", seen)
	}
}
