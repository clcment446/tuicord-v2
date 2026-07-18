//go:build !windows

package term

import (
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func queryTerminal(fd int, timeout time.Duration) string {
	oldFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return ""
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		return ""
	}
	defer unix.FcntlInt(uintptr(fd), unix.F_SETFL, oldFlags)

	_, _ = unix.Write(fd, []byte("\x1b[?u\x1b[?2026$p"))
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 128)
	var out strings.Builder
	for time.Now().Before(deadline) {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			out.Write(buf[:n])
			if strings.Contains(out.String(), "$y") {
				break
			}
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			time.Sleep(time.Millisecond)
			continue
		}
		if err != nil {
			break
		}
	}
	return out.String()
}
