//go:build windows

package term

import (
	"os"

	xterm "golang.org/x/term"
)

func notifyResize(chan<- os.Signal) {
	// Windows consoles do not expose SIGWINCH. Size changes are discovered by
	// callers when they query the terminal.
}

func terminalSize(fd int) (Size, error) {
	width, height, err := xterm.GetSize(fd)
	if err != nil {
		return Size{}, err
	}
	return Size{Width: width, Height: height}, nil
}
