package term

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/tui/screen"
	"golang.org/x/sys/unix"
)

func probeCapabilities(fd int, environ []string, timeout time.Duration) Capabilities {
	env := envMap(environ)
	caps := capabilitiesFromEnv(env)
	if isKitty(env) {
		if palette, ok := loadKittyANSI16Palette(kittyThemePath(env)); ok {
			caps.ANSI16 = palette
			caps.ANSI16Known = true
		}
	}
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

func isKitty(env map[string]string) bool {
	return strings.Contains(strings.ToLower(env["TERM"]), "kitty") || strings.Contains(strings.ToLower(env["TERM_PROGRAM"]), "kitty")
}

func kittyThemePath(env map[string]string) string {
	configHome := env["XDG_CONFIG_HOME"]
	if configHome == "" {
		configHome = filepath.Join(env["HOME"], ".config")
	}
	return filepath.Join(configHome, "kitty", "current-theme.conf")
}

func loadKittyANSI16Palette(path string) (screen.Palette, bool) {
	file, err := os.Open(path)
	if err != nil {
		return screen.Palette{}, false
	}
	defer file.Close()

	palette := screen.DefaultANSI16Palette()
	seen := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || !strings.HasPrefix(fields[0], "color") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(fields[0], "color"))
		if err != nil || index < 0 || index >= len(palette) {
			continue
		}
		color, err := parseKittyHexColor(fields[1])
		if err != nil {
			continue
		}
		palette[index] = color
		seen++
	}
	if err := scanner.Err(); err != nil || seen == 0 {
		return screen.Palette{}, false
	}
	return palette, true
}

func parseKittyHexColor(value string) (screen.Color, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return screen.Color{}, strconv.ErrSyntax
	}
	v, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return screen.Color{}, err
	}
	return screen.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
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
