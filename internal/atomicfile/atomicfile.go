// Package atomicfile writes files through a synced temporary sibling before
// atomically replacing the destination.
package atomicfile

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write creates path's parent directory, writes the file through a temporary
// sibling, syncs and closes it, then atomically replaces path. The temporary
// file is removed after every unsuccessful write.
func Write(path string, perm fs.FileMode, encode func(io.Writer) error) error {
	if encode == nil {
		return fmt.Errorf("write %s: nil encoder", path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("set permissions on temporary file for %s: %w", path, err)
	}
	if err := encode(tmp); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	closed = true
	if err := replaceFile(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
