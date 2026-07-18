// Package atomicfile writes files through a synced temporary sibling before
// atomically installing them at their destination.
package atomicfile

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write creates path's parent directory, writes the file through a temporary
// sibling, syncs and closes it, then atomically replaces path. On Unix systems
// that support it, Write also makes newly created parent directories and the
// replacement crash-durable by syncing the affected directories. The temporary
// file is removed after every unsuccessful write.
func Write(path string, perm fs.FileMode, encode func(io.Writer) error) error {
	return write(path, perm, encode, syncParentDirectory)
}

// WriteNew is like Write, but atomically installs the file only when path does
// not exist. It returns an error matching fs.ErrExist when another creator wins;
// it never replaces the winning file.
func WriteNew(path string, perm fs.FileMode, encode func(io.Writer) error) error {
	return writeWithInstaller(path, perm, encode, syncParentDirectory, installFileNoReplace, "create")
}

func write(path string, perm fs.FileMode, encode func(io.Writer) error, syncParent func(string) error) error {
	return writeWithInstaller(path, perm, encode, syncParent, replaceFile, "replace")
}

func writeWithInstaller(
	path string,
	perm fs.FileMode,
	encode func(io.Writer) error,
	syncParent func(string) error,
	install func(string, string) error,
	installVerb string,
) error {
	if encode == nil {
		return fmt.Errorf("write %s: nil encoder", path)
	}
	dir := filepath.Dir(path)
	if err := makeParentDirectories(dir, syncParent); err != nil {
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
	if err := install(tmpName, path); err != nil {
		return fmt.Errorf("%s %s: %w", installVerb, path, err)
	}
	if err := syncParent(dir); err != nil {
		return fmt.Errorf("sync parent directory for %s: %w", path, err)
	}
	return nil
}

// makeParentDirectories is deliberately incremental rather than using
// os.MkdirAll. Each new directory entry is committed by syncing its parent;
// the final directory is synced after the file is installed. That ordering
// makes an entire newly created path durable, not only its last rename.
func makeParentDirectories(dir string, syncParent func(string) error) error {
	dir = filepath.Clean(dir)
	missing := make([]string, 0, 4)
	current := dir
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
		current = parent
	}

	for i := len(missing) - 1; i >= 0; i-- {
		path := missing[i]
		if err := os.Mkdir(path, 0o755); err != nil {
			// Another writer may create the same directory after the discovery
			// walk. Accept only a directory, then sync its parent on our behalf.
			info, statErr := os.Stat(path)
			if !os.IsExist(err) || statErr != nil || !info.IsDir() {
				return err
			}
		}
		parent := filepath.Dir(path)
		if err := syncParent(parent); err != nil {
			return fmt.Errorf("sync %s after creating %s: %w", parent, path, err)
		}
	}
	return nil
}
