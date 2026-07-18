//go:build linux

package media

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	ipcPath    string
	shmMu      sync.Mutex
	shmFiles   []string
}

var videoIPCSequence atomic.Uint64

// NewVideoPlayer returns a player configured from cfg.
func NewVideoPlayer(cfg Config) *VideoPlayer { return &VideoPlayer{cfg: cfg} }

// Available reports whether inline video can run: the mpv binary must resolve on
// PATH (or as an absolute path).
func (p *VideoPlayer) Available() bool {
	if p == nil {
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
	if p == nil {
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

	session := &videoSession{url: url, region: region, cmd: cmd, master: master, readerDone: make(chan struct{}), ipcPath: ipcPath}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()

	// Forward mpv's graphics bytes to the terminal.
	go func() {
		defer close(session.readerDone)
		buf := make([]byte, 32*1024)
		var framer kittyOutputFramer
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
					if stable, path, stableErr := stabilizeKittySHM(chunk, "/dev/shm"); stableErr == nil {
						chunk = stable
						if path != "" {
							session.trackSHM(path)
						}
					}
					emit(chunk)
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

func (s *videoSession) trackSHM(path string) {
	s.shmMu.Lock()
	defer s.shmMu.Unlock()
	s.shmFiles = append(s.shmFiles, path)
	// A conforming terminal unlinks consumed objects. Bound leftovers for
	// terminals that do not, while retaining several seconds of frame history.
	if len(s.shmFiles) > 120 {
		_ = os.Remove(s.shmFiles[0])
		s.shmFiles = s.shmFiles[1:]
	}
}

func (s *videoSession) cleanupSHM() {
	s.shmMu.Lock()
	files := append([]string(nil), s.shmFiles...)
	s.shmFiles = nil
	s.shmMu.Unlock()
	for _, path := range files {
		_ = os.Remove(path)
	}
}

// stabilizeKittySHM snapshots an mpv t=s frame and rewrites its Kitty payload
// to a unique object name. mpv reuses its source object; forwarding that name
// directly lets it overwrite the pixels before the real terminal consumes the
// queued command, especially across pause/resume.
func stabilizeKittySHM(packet []byte, dir string) ([]byte, string, error) {
	start := bytes.Index(packet, kittyAPCStart)
	if start < 0 {
		return packet, "", nil
	}
	end := bytes.Index(packet[start+len(kittyAPCStart):], kittyAPCEnd)
	if end < 0 {
		return packet, "", nil
	}
	end += start + len(kittyAPCStart)
	body := packet[start+len(kittyAPCStart) : end]
	semicolon := bytes.IndexByte(body, ';')
	if semicolon < 0 || !bytes.Contains(body[:semicolon], []byte("t=s")) {
		return packet, "", nil
	}
	nameBytes, err := base64.StdEncoding.DecodeString(string(body[semicolon+1:]))
	if err != nil {
		return nil, "", err
	}
	sourceName := string(nameBytes)
	source := sourceName
	if !filepath.IsAbs(source) {
		source = filepath.Join(dir, source)
	}
	pixels, err := os.ReadFile(source)
	if err != nil {
		return nil, "", err
	}
	stableName := fmt.Sprintf("tuicord-kitty-%d-%d", os.Getpid(), videoIPCSequence.Add(1))
	stablePath := filepath.Join(dir, stableName)
	if err := os.WriteFile(stablePath, pixels, 0o600); err != nil {
		return nil, "", err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(stableName))
	rewritten := make([]byte, 0, len(packet)+len(encoded))
	rewritten = append(rewritten, packet[:start+len(kittyAPCStart)+semicolon+1]...)
	rewritten = append(rewritten, encoded...)
	rewritten = append(rewritten, packet[end:]...)
	return rewritten, stablePath, nil
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
		_ = session.cmd.Process.Kill()
	}
	_ = session.master.Close()
	_ = os.Remove(session.ipcPath)
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
