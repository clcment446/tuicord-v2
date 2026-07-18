//go:build !windows

package term

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
	xterm "golang.org/x/term"
)

func notifyResize(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}

func terminalSize(fd int) (Size, error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		// Fall back to x/term so a platform without the ioctl still reports cells.
		w, h, gerr := xterm.GetSize(fd)
		if gerr != nil {
			return Size{}, err
		}
		return Size{Width: w, Height: h}, nil
	}
	return Size{
		Width:  int(ws.Col),
		Height: int(ws.Row),
		XPixel: int(ws.Xpixel),
		YPixel: int(ws.Ypixel),
	}, nil
}
