//go:build linux

package media

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// VideoPlayer plays one video at a time inside the terminal by driving mpv with
// its Kitty graphics video output on a pseudo-terminal. mpv's output bytes are
// read from the pty and handed to a sink that the UI writes to the real
// terminal, positioned at the target cell region via --vo-kitty-left/top.
//
// A VideoPlayer is safe for concurrent use. All methods are cheap; the actual
// I/O runs on background goroutines.
type VideoPlayer struct {
	cfg Config

	mu      sync.Mutex
	session *videoSession
}

type videoSession struct {
	url        string
	region     Rect
	cmd        *exec.Cmd
	master     *os.File
	readerDone chan struct{}
	done       chan struct{}
	ipcPath    string
	shmMu      sync.Mutex
	shmFiles   []shmSnapshot
	shmBytes   int64
}

type shmSnapshot struct {
	path  string
	bytes int64
}

var videoIPCSequence atomic.Uint64

// NewVideoPlayer returns a player configured from cfg.
func NewVideoPlayer(cfg Config) *VideoPlayer { return &VideoPlayer{cfg: cfg} }

// Available reports whether inline video can run: the mpv binary must resolve on
// PATH (or as an absolute path).
func (p *VideoPlayer) Available() bool {
	if p == nil || !p.cfg.VideoEnabled {
		return false
	}
	_, err := exec.LookPath(p.mpvPath())
	return err == nil
}

func (p *VideoPlayer) mpvPath() string {
	if p.cfg.MpvPath != "" {
		return p.cfg.MpvPath
	}
	return "mpv"
}

// Playing returns the URL currently playing, if any.
func (p *VideoPlayer) Playing() (string, bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session == nil {
		return "", false
	}
	return p.session.url, true
}

// Play starts (or replaces) playback of url in region. out receives raw terminal
// bytes to forward to the real terminal; it must be safe to call from a
// background goroutine and should serialize the bytes with the app's own frame
// writes. onExit runs once when this playback ends on its own or is stopped, on
// a background goroutine.
//
// The pty is sized to the region: mpv's kitty vo clears its whole "terminal"
// before each frame, so a pty the size of the full screen makes it paint the
// entire screen black. A region-sized pty keeps that clear confined to the
// playback area.
func (p *VideoPlayer) Play(url string, region Rect, out func([]byte), onExit func()) error {
	if p == nil || !p.cfg.VideoEnabled {
		return ErrVideoUnsupported
	}
	if region.Empty() {
		return fmt.Errorf("media: play %s: empty region", url)
	}
	p.Stop()

	master, slaveName, err := openPTY()
	if err != nil {
		return err
	}
	if err := setPTYSize(master, region.Cols, region.Rows); err != nil {
		_ = master.Close()
		return err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR, 0)
	if err != nil {
		_ = master.Close()
		return fmt.Errorf("media: open pty slave: %w", err)
	}

	ipcPath := filepath.Join(os.TempDir(), fmt.Sprintf("tuicord-mpv-%d-%d.sock", os.Getpid(), videoIPCSequence.Add(1)))
	_ = os.Remove(ipcPath)
	cmd := exec.Command(p.mpvPath(), p.args(url, region, ipcPath)...)
	// mpv's Kitty video output keys off TERM; force it so the child emits Kitty
	// graphics regardless of the parent's terminal name (e.g. xterm-ghostty).
	cmd.Env = append(os.Environ(), "TERM=xterm-kitty")
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}

	if err := cmd.Start(); err != nil {
		_ = slave.Close()
		_ = master.Close()
		return fmt.Errorf("media: start mpv: %w", err)
	}
	// The child owns the slave now; the parent keeps only the master.
	_ = slave.Close()

	session := &videoSession{url: url, region: region, cmd: cmd, master: master, readerDone: make(chan struct{}), done: make(chan struct{}), ipcPath: ipcPath}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()

	// Forward mpv's graphics bytes to the terminal.
	go func() {
		defer close(session.readerDone)
		buf := make([]byte, 32*1024)
		var framer kittyOutputFramer
		bounded := p.cfg.Bounded()
		frameMaxBytes := min(bounded.MaxResponseBytes, bounded.DecodedCacheMaxBytes)
		retainedMaxBytes := bounded.DecodedCacheMaxBytes
		emit := func(chunk []byte) {
			if len(chunk) == 0 || out == nil {
				return
			}
			p.mu.Lock()
			current := p.session == session
			p.mu.Unlock()
			if current {
				out(chunk)
			}
		}
		for {
			n, err := master.Read(buf)
			if n > 0 {
				for _, chunk := range framer.Push(buf[:n]) {
					stable, ok := session.snapshotKittyChunk(chunk, "/dev/shm", frameMaxBytes, retainedMaxBytes)
					if !ok {
						// Framing gives us the complete APC command. Dropping it whole on
						// an oversized or unreadable SHM object keeps later terminal bytes
						// outside Kitty's payload parser.
						continue
					}
					emit(stable)
				}
			}
			if err != nil {
				emit(framer.Flush())
				return
			}
		}
	}()

	// Reap the process and notify when it ends by itself.
	go func() {
		defer close(session.done)
		_ = cmd.Wait()
		_ = master.Close()
		// Ensure mpv's final Kitty delete has been queued before notifying the UI
		// to repaint the widget tree.
		<-session.readerDone
		_ = os.Remove(session.ipcPath)
		session.cleanupSHM()
		p.mu.Lock()
		ended := p.session == session
		if ended {
			p.session = nil
		}
		p.mu.Unlock()
		if ended && onExit != nil {
			onExit()
		}
	}()

	return nil
}

func (s *videoSession) snapshotKittyChunk(chunk []byte, dir string, frameMaxBytes, retainedMaxBytes int64) ([]byte, bool) {
	stable, path, size, err := stabilizeKittySHMWithLimit(chunk, dir, frameMaxBytes)
	if err != nil {
		return nil, false
	}
	if path != "" && !s.trackSHM(path, size, retainedMaxBytes) {
		return nil, false
	}
	return stable, true
}

// trackSHM retains a snapshot only after evicting enough old snapshots to stay
// within the byte budget. A conforming terminal unlinks consumed objects;
// Remove remains harmless in that case. It returns false and removes path when
// one frame cannot fit the total budget.
func (s *videoSession) trackSHM(path string, size, maxBytes int64) bool {
	if path == "" {
		return true
	}
	if size < 0 || maxBytes <= 0 || size > maxBytes {
		_ = os.Remove(path)
		return false
	}

	s.shmMu.Lock()
	var evicted []shmSnapshot
	const maxSnapshotFiles = 120
	for len(s.shmFiles) > 0 && (len(s.shmFiles) >= maxSnapshotFiles || s.shmBytes > maxBytes-size) {
		evicted = append(evicted, s.shmFiles[0])
		s.shmBytes -= s.shmFiles[0].bytes
		s.shmFiles = s.shmFiles[1:]
	}
	s.shmFiles = append(s.shmFiles, shmSnapshot{path: path, bytes: size})
	s.shmBytes += size
	s.shmMu.Unlock()

	for _, snapshot := range evicted {
		_ = os.Remove(snapshot.path)
	}
	return true
}

func (s *videoSession) cleanupSHM() {
	s.shmMu.Lock()
	files := append([]shmSnapshot(nil), s.shmFiles...)
	s.shmFiles = nil
	s.shmBytes = 0
	s.shmMu.Unlock()
	for _, snapshot := range files {
		_ = os.Remove(snapshot.path)
	}
}

// stabilizeKittySHM snapshots an mpv t=s frame with the default media response
// limit. Keep this compatibility seam for package callers and tests; playback
// passes its configured limit to stabilizeKittySHMWithLimit.
func stabilizeKittySHM(packet []byte, dir string) ([]byte, string, error) {
	stable, path, _, err := stabilizeKittySHMWithLimit(packet, dir, DefaultMaxResponseBytes)
	return stable, path, err
}

// stabilizeKittySHMWithLimit snapshots an mpv t=s frame and rewrites its Kitty
// payload to a unique object name. Source reads are capped at maxBytes+1 and the
// destination receives at most maxBytes, so a malicious or raced SHM object can
// never cause an unbounded allocation or write.
func stabilizeKittySHMWithLimit(packet []byte, dir string, maxBytes int64) ([]byte, string, int64, error) {
	start := bytes.Index(packet, kittyAPCStart)
	if start < 0 {
		return packet, "", 0, nil
	}
	end := bytes.Index(packet[start+len(kittyAPCStart):], kittyAPCEnd)
	if end < 0 {
		return packet, "", 0, nil
	}
	end += start + len(kittyAPCStart)
	body := packet[start+len(kittyAPCStart) : end]
	semicolon := bytes.IndexByte(body, ';')
	if semicolon < 0 || !bytes.Contains(body[:semicolon], []byte("t=s")) {
		return packet, "", 0, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}
	payload := body[semicolon+1:]
	// POSIX SHM names are tiny. Bounding the decoded name also avoids allocating
	// from an arbitrarily large malformed Kitty command.
	const maxSHMNameBytes = 4096
	if len(payload) > base64.StdEncoding.EncodedLen(maxSHMNameBytes) {
		return nil, "", 0, fmt.Errorf("media: mpv shared-memory name is too long")
	}
	nameBytes := make([]byte, base64.StdEncoding.DecodedLen(len(payload)))
	n, err := base64.StdEncoding.Decode(nameBytes, payload)
	if err != nil {
		return nil, "", 0, err
	}
	nameBytes = nameBytes[:n]
	if len(nameBytes) == 0 || len(nameBytes) > maxSHMNameBytes || bytes.IndexByte(nameBytes, 0) >= 0 {
		return nil, "", 0, fmt.Errorf("media: invalid mpv shared-memory name")
	}
	sourceName := string(nameBytes)
	source := sourceName
	if !filepath.IsAbs(source) {
		source = filepath.Join(dir, source)
	}
	src, err := os.Open(source)
	if err != nil {
		return nil, "", 0, err
	}
	defer src.Close()

	stableName := fmt.Sprintf("tuicord-kitty-%d-%d", os.Getpid(), videoIPCSequence.Add(1))
	stablePath := filepath.Join(dir, stableName)
	dst, err := os.OpenFile(stablePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, "", 0, err
	}
	keep := false
	defer func() {
		_ = dst.Close()
		if !keep {
			_ = os.Remove(stablePath)
		}
	}()

	written, copyErr := io.CopyN(dst, src, maxBytes)
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return nil, "", 0, copyErr
	}
	if written == maxBytes {
		var extra [1]byte
		extraN, readErr := src.Read(extra[:])
		if extraN > 0 {
			return nil, "", 0, fmt.Errorf("%w: limit is %d bytes", ErrVideoFrameTooLarge, maxBytes)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, "", 0, readErr
		}
	}
	if err := dst.Close(); err != nil {
		return nil, "", 0, err
	}
	keep = true

	encoded := base64.StdEncoding.EncodeToString([]byte(stableName))
	rewritten := make([]byte, 0, len(packet)+len(encoded))
	rewritten = append(rewritten, packet[:start+len(kittyAPCStart)+semicolon+1]...)
	rewritten = append(rewritten, encoded...)
	rewritten = append(rewritten, packet[end:]...)
	return rewritten, stablePath, written, nil
}

// Stop ends any current playback. It does not clear mpv's final frame from the
// screen; the caller should write KittyClearRegion for the region afterwards.
func (p *VideoPlayer) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	session := p.session
	p.session = nil
	p.mu.Unlock()
	if session == nil {
		return
	}
	if session.cmd.Process != nil {
		// mpv owns a session/process group and may launch helpers for remote
		// streams. Kill the group first, then the leader as a portable fallback.
		_ = syscall.Kill(-session.cmd.Process.Pid, syscall.SIGKILL)
		_ = session.cmd.Process.Kill()
	}
	_ = session.master.Close()
	_ = os.Remove(session.ipcPath)
	select {
	case <-session.done:
	case <-time.After(2 * time.Second):
		// Process.Kill has been issued; avoid making UI shutdown hang forever on
		// a platform/kernel edge while still bounding cleanup wait time.
	}
}

// args builds mpv's command line for inline Kitty playback pinned to region.
func (p *VideoPlayer) args(url string, r Rect, ipcPath string) []string {
	args := []string{
		"--no-config",
		"--really-quiet",
		"--no-input-terminal",
		"--input-vo-keyboard=no",
		"--loop=no",
		"--keep-open=yes",
		"--input-ipc-server=" + ipcPath,
		"--vo=kitty",
		"--vo-kitty-alt-screen=no",
		"--vo-kitty-config-clear=no",
		fmt.Sprintf("--vo-kitty-use-shm=%s", yesno(p.cfg.VideoUseSHM)),
		fmt.Sprintf("--vo-kitty-cols=%d", r.Cols),
		fmt.Sprintf("--vo-kitty-rows=%d", r.Rows),
		fmt.Sprintf("--vo-kitty-width=%d", r.Cols*cellWidth(p.cfg)),
		fmt.Sprintf("--vo-kitty-height=%d", r.Rows*cellHeight(p.cfg)),
		// Pin placement. Automatic (zero) positioning depends on the terminal's
		// current cursor, which the UI moves whenever it redraws the controls and
		// therefore makes later video frames jump into unrelated rows.
		fmt.Sprintf("--vo-kitty-left=%d", r.X+1),
		fmt.Sprintf("--vo-kitty-top=%d", r.Y+1),
	}
	if p.cfg.RequestTimeout > 0 {
		args = append(args, fmt.Sprintf("--network-timeout=%.3f", p.cfg.RequestTimeout.Seconds()))
	}
	if !p.cfg.VideoAudio {
		args = append(args, "--no-audio")
	}
	return append(args, url)
}

func (p *VideoPlayer) currentSession() *videoSession {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session
}

func (p *VideoPlayer) ipc(commands ...[]any) ([]any, error) {
	session := p.currentSession()
	if session == nil {
		return nil, ErrVideoUnsupported
	}
	conn, err := net.DialTimeout("unix", session.ipcPath, 80*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(120 * time.Millisecond))
	enc := json.NewEncoder(conn)
	for _, command := range commands {
		if err := enc.Encode(map[string]any{"command": command}); err != nil {
			return nil, err
		}
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	values := make([]any, 0, len(commands))
	for range commands {
		var response struct {
			Error string `json:"error"`
			Data  any    `json:"data"`
		}
		if err := dec.Decode(&response); err != nil {
			return nil, err
		}
		if response.Error != "success" {
			return nil, fmt.Errorf("mpv: %s", response.Error)
		}
		values = append(values, response.Data)
	}
	return values, nil
}

func (p *VideoPlayer) TogglePause() error {
	_, err := p.ipc([]any{"cycle", "pause"})
	return err
}

func (p *VideoPlayer) Replay() error {
	_, err := p.ipc([]any{"seek", 0, "absolute+exact"}, []any{"set_property", "pause", false})
	return err
}

func (p *VideoPlayer) SeekPercent(percent float64) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	_, err := p.ipc([]any{"seek", percent, "absolute-percent+exact"})
	return err
}

func (p *VideoPlayer) SeekRelative(seconds float64) error {
	_, err := p.ipc([]any{"seek", seconds, "relative+exact"})
	return err
}

// Resize updates the active PTY and Kitty VO geometry without restarting
// playback, preserving pause state and the current timestamp.
func (p *VideoPlayer) Resize(region Rect) error {
	if region.Empty() {
		return fmt.Errorf("media: resize video: empty region")
	}
	p.mu.Lock()
	session := p.session
	if session != nil {
		session.region = region
	}
	p.mu.Unlock()
	if session == nil {
		return ErrVideoUnsupported
	}
	if err := setPTYSize(session.master, region.Cols, region.Rows); err != nil {
		return err
	}
	if session.cmd.Process != nil {
		_ = session.cmd.Process.Signal(syscall.SIGWINCH)
	}
	w, h := p.cfg.CellPixels()
	_, err := p.ipc(
		[]any{"set_property", "options/vo-kitty-cols", region.Cols},
		[]any{"set_property", "options/vo-kitty-rows", region.Rows},
		[]any{"set_property", "options/vo-kitty-width", region.Cols * w},
		[]any{"set_property", "options/vo-kitty-height", region.Rows * h},
		[]any{"set_property", "options/vo-kitty-left", region.X + 1},
		[]any{"set_property", "options/vo-kitty-top", region.Y + 1},
	)
	return err
}

func (p *VideoPlayer) Status() (VideoStatus, error) {
	values, err := p.ipc(
		[]any{"get_property", "time-pos"},
		[]any{"get_property", "duration"},
		[]any{"get_property", "pause"},
		[]any{"get_property", "eof-reached"},
	)
	if err != nil {
		return VideoStatus{}, err
	}
	status := VideoStatus{}
	if v, ok := values[0].(float64); ok {
		status.Position = v
	}
	if v, ok := values[1].(float64); ok {
		status.Duration = v
	}
	if v, ok := values[2].(bool); ok {
		status.Paused = v
	}
	if v, ok := values[3].(bool); ok {
		status.Ended = v
	}
	return status, nil
}

func cellWidth(cfg Config) int  { w, _ := cfg.CellPixels(); return w }
func cellHeight(cfg Config) int { _, h := cfg.CellPixels(); return h }

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
