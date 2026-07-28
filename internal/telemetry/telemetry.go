// Package telemetry records bounded, redacted diagnostics locally for support
// exports. It never sends data anywhere.
package telemetry

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxBodyBytes = 64 << 10
	maxEvents    = 10000
)

type PermissionSnapshot struct {
	ChannelID   uint64 `json:"channel_id"`
	GuildID     uint64 `json:"guild_id,omitempty"`
	Known       bool   `json:"known"`
	Permissions uint64 `json:"permissions,omitempty"`
	CanRead     bool   `json:"can_read"`
	CanSend     bool   `json:"can_send"`
	CanManage   bool   `json:"can_manage"`
}

type PermissionResolver func([]uint64) []PermissionSnapshot

type Recorder struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	started   time.Time
	sessionID string
	events    int
	resolver  PermissionResolver
}

type Event struct {
	Time            time.Time            `json:"time"`
	SessionID       string               `json:"sessionID"`
	Kind            string               `json:"kind"`
	Method          string               `json:"method,omitempty"`
	Endpoint        string               `json:"endpoint,omitempty"`
	Reason          string               `json:"reason,omitempty"`
	Channels        []uint64             `json:"channels,omitempty"`
	Permissions     []PermissionSnapshot `json:"permissions,omitempty"`
	Headers         map[string]string    `json:"headers,omitempty"`
	ResponseHeaders map[string]string    `json:"response_headers,omitempty"`
	Body            string               `json:"body,omitempty"`
	BodyBytes       int                  `json:"body_bytes,omitempty"`
	Status          int                  `json:"status,omitempty"`
	Error           string               `json:"error,omitempty"`
	LatencyMS       int64                `json:"latency_ms,omitempty"`
	Fields          map[string]any       `json:"fields,omitempty"`
}

func New(dir, token string) (*Recorder, error) {
	if dir == "" {
		return nil, errors.New("telemetry: empty directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create telemetry directory: %w", err)
	}
	now := time.Now().UTC()
	path := filepath.Join(dir, "session-"+now.Format("20060102T150405.000000000Z")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open telemetry session: %w", err)
	}
	r := &Recorder{file: f, path: path, started: now, sessionID: strconv.FormatInt(now.Unix(), 10) + "-" + hash([]byte(token))}
	_ = r.record(Event{Kind: "session_start", Fields: map[string]any{"version": 1}})
	return r, nil
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.record(Event{Kind: "session_end", Fields: map[string]any{"duration_ms": time.Since(r.started).Milliseconds()}})
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}
func (r *Recorder) SessionID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}
func (r *Recorder) SetPermissionResolver(fn PermissionResolver) {
	if r != nil {
		r.mu.Lock()
		r.resolver = fn
		r.mu.Unlock()
	}
}

func (r *Recorder) Record(e Event) {
	if r != nil {
		_ = r.record(e)
	}
}

func (r *Recorder) record(e Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil || r.events >= maxEvents {
		return nil
	}
	e.Time = e.Time.UTC()
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Body != "" {
		e.Body, _ = RedactBody([]byte(e.Body))
	}
	e.SessionID = r.sessionID
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = r.file.Write(append(b, '\n')); err != nil {
		return err
	}
	r.events++
	return nil
}

func (r *Recorder) Permissions(channels []uint64) []PermissionSnapshot {
	if r == nil || len(channels) == 0 {
		return nil
	}
	r.mu.Lock()
	fn := r.resolver
	r.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(channels)
}

func ExportLatest(dir, destination string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read telemetry directory: %w", err)
	}
	var latest string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "session-") && strings.HasSuffix(e.Name(), ".jsonl") && (latest == "" || e.Name() > latest) {
			latest = e.Name()
		}
	}
	if latest == "" {
		return errors.New("telemetry: no session found")
	}
	return exportFiles(destination, filepath.Join(dir, latest))
}

func exportFiles(destination, session string) (err error) {
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	zw := zip.NewWriter(f)
	defer func() {
		if closeErr := zw.Close(); err == nil {
			err = closeErr
		}
	}()
	manifest, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err = io.WriteString(manifest, `{"format":1,"token_included":false,"local_only":true}`); err != nil {
		return err
	}
	in, err := os.Open(session)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := zw.Create("events.jsonl")
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}

var sensitiveKey = regexp.MustCompile(`(?i)(authorization|token|cookie|password|secret|credential|access[_-]?key|api[_-]?key)`)

func RedactHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, values := range h {
		if sensitiveKey.MatchString(k) {
			out[k] = "<redacted>"
		} else if len(values) > 0 {
			out[k] = values[0]
		}
	}
	return out
}

func RedactBody(data []byte) (string, int) {
	original := len(data)
	if len(data) > maxBodyBytes {
		data = data[:maxBodyBytes]
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		value = redactJSON(value)
		data, _ = json.Marshal(value)
	} else if len(data) > 0 {
		data = []byte("<non-json body sha256=" + hash(data) + ">")
	}
	if len(data) > maxBodyBytes {
		data = append(data[:maxBodyBytes], []byte("…<truncated>")...)
	}
	return string(data), original
}

func redactJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if sensitiveKey.MatchString(k) {
				out[k] = "<redacted>"
			} else {
				out[k] = redactJSON(v)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = redactJSON(v)
		}
		return out
	default:
		return v
	}
}

func hash(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

func Endpoint(reqURL *url.URL) string {
	if reqURL == nil {
		return ""
	}
	path := reqURL.Path
	if i := strings.Index(path, "/api/"); i >= 0 {
		path = path[i:]
	}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err == nil && p != "" {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func ChannelIDs(reqURL *url.URL) []uint64 {
	if reqURL == nil {
		return nil
	}
	parts := strings.Split(reqURL.Path, "/")
	out := make([]uint64, 0, 2)
	for i, p := range parts {
		if p != "channels" || i+1 >= len(parts) {
			continue
		}
		id, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err == nil {
			out = append(out, id)
		}
	}
	return out
}

func Reason(endpoint string) string {
	switch {
	case strings.Contains(endpoint, "/messages"):
		return "message_history_or_send"
	case strings.Contains(endpoint, "/channels"):
		return "channel_directory_or_detail"
	case strings.Contains(endpoint, "/guilds"):
		return "guild_directory_or_metadata"
	case strings.Contains(endpoint, "application-command"):
		return "command_catalog_or_interaction"
	default:
		return "other_rest_request"
	}
}
