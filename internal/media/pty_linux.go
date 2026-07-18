//go:build linux

package media

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal pair and returns the master file and the
// slave device path. The caller opens the slave, wires it to a child process's
// standard descriptors, and closes it in the parent once the child has started.
func openPTY() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, "", fmt.Errorf("media: open ptmx: %w", err)
	}
	// Unlock the slave so it can be opened.
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, "", fmt.Errorf("media: unlock pty: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, "", fmt.Errorf("media: pty number: %w", err)
	}
	return master, fmt.Sprintf("/dev/pts/%d", n), nil
}

// setPTYSize sets a pty's window size in character cells so the child sees the
// region it should render into.
func setPTYSize(f *os.File, cols, rows int) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	})
}
