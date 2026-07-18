//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package atomicfile

import "os"

// syncParentDirectory makes the rename durable across a crash once the file
// data itself has been synced. Directory fsync is supported on these Unix
// platforms.
func syncParentDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
