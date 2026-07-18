//go:build !windows

package atomicfile

import "os"

// installFileNoReplace links the already-synced sibling into place. A hard
// link is an atomic no-clobber operation on the Unix filesystems supported by
// this package. Removing the temporary name before the directory sync commits
// both directory-entry changes together.
func installFileNoReplace(oldPath, newPath string) error {
	if err := os.Link(oldPath, newPath); err != nil {
		return err
	}
	return os.Remove(oldPath)
}
