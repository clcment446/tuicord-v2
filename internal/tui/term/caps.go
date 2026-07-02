package term

import (
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func probeCapabilities(fd int, environ []string, timeout time.Duration) Capabilities {
	env := envMap(environ)
	caps := capabilitiesFromEnv(env)
	if fd >= 0 && timeout > 0 {
		reply := queryTerminal(fd, timeout)
		if strings.Contains(reply, "?2026;1$y") || strings.Contains(reply, "?2026;2$y") {
			caps.SyncOutput = true
		}
		if strings.Contains(reply, "?") && strings.Contains(reply, "u") {
			caps.KittyKeyboard = true
		}
	}
	return caps
}

func capabilitiesFromEnv(env map[string]string) Capabilities {
	termName := strings.ToLower(env["TERM"])
	termProgram := strings.ToLower(env["TERM_PROGRAM"])
	colorTerm := strings.ToLower(env["COLORTERM"])
	noColor := env["NO_COLOR"] != ""

	caps := Capabilities{
		Color256: strings.Contains(termName, "256color") ||
			strings.Contains(termName, "direct") ||
			strings.Contains(termName, "truecolor"),
		TrueColor: !noColor && (strings.Contains(colorTerm, "truecolor") ||
			strings.Contains(colorTerm, "24bit") ||
			strings.Contains(termName, "direct") ||
			strings.Contains(termName, "truecolor")),
		KittyKeyboard: strings.Contains(termName, "kitty") ||
			strings.Contains(termProgram, "kitty") ||
			strings.Contains(termProgram, "wezterm"),
		SyncOutput: strings.Contains(termName, "kitty") ||
			strings.Contains(termProgram, "kitty") ||
			strings.Contains(termProgram, "wezterm"),
	}
	if noColor {
		caps.TrueColor = false
	}
	return caps
}

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
