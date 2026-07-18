//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

// installFileNoReplace uses a same-directory move without
// MOVEFILE_REPLACE_EXISTING. It therefore remains atomic while preserving a
// destination created by another process.
func installFileNoReplace(oldPath, newPath string) error {
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, 0)
}
