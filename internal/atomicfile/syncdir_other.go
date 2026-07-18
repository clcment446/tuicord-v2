//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package atomicfile

// Some platforms do not support syncing an open directory. The file itself is
// still synced before replacement there.
func syncParentDirectory(string) error { return nil }
