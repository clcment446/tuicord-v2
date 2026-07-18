//go:build linux

package media

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
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
	url    string
	region Rect
	cmd    *exec.Cmd
	master *os.File
}

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

	cmd := exec.Command(p.mpvPath(), p.args(url, region)...)
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

	session := &videoSession{url: url, region: region, cmd: cmd, master: master}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()

	// Forward mpv's graphics bytes to the terminal.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := master.Read(buf)
			if n > 0 && out != nil {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				out(chunk)
			}
			if err != nil {
				return
			}
		}
	}()

	// Reap the process and notify when it ends by itself.
	go func() {
		_ = cmd.Wait()
		_ = master.Close()
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
}

// args builds mpv's command line for inline Kitty playback pinned to region.
func (p *VideoPlayer) args(url string, r Rect) []string {
	args := []string{
		"--no-config",
		"--really-quiet",
		"--no-input-terminal",
		"--input-vo-keyboard=no",
		"--loop=no",
		"--keep-open=no",
		"--vo=kitty",
		"--vo-kitty-alt-screen=no",
		fmt.Sprintf("--vo-kitty-use-shm=%s", yesno(p.cfg.VideoUseSHM)),
		fmt.Sprintf("--vo-kitty-cols=%d", r.Cols),
		fmt.Sprintf("--vo-kitty-rows=%d", r.Rows),
		// mpv addresses cells from 1; our Rect is 0-based.
		fmt.Sprintf("--vo-kitty-left=%d", r.X+1),
		fmt.Sprintf("--vo-kitty-top=%d", r.Y+1),
	}
	if !p.cfg.VideoAudio {
		args = append(args, "--no-audio")
	}
	return append(args, url)
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
