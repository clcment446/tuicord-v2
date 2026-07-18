package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesParentAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.toml")
	if err := Write(path, 0o640, func(w io.Writer) error {
		_, err := io.WriteString(w, "new contents")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new contents" {
		t.Fatalf("contents = %q, want new contents", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestWriteSyncsParentAfterRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	synced := false
	if err := write(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "durable")
		return err
	}, func(gotDir string) error {
		if gotDir != dir {
			t.Fatalf("synced directory = %q, want %q", gotDir, dir)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("destination was not renamed before directory sync: %v", err)
		}
		if string(contents) != "durable" {
			t.Fatalf("contents at directory sync = %q", contents)
		}
		synced = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("parent directory was not synced")
	}
}

func TestWriteReportsParentSyncFailure(t *testing.T) {
	wantErr := errors.New("directory sync failed")
	path := filepath.Join(t.TempDir(), "state.toml")
	err := write(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "renamed")
		return err
	}, func(string) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
	if contents, readErr := os.ReadFile(path); readErr != nil || string(contents) != "renamed" {
		t.Fatalf("renamed destination = %q, %v", contents, readErr)
	}
}

func TestWriteReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, 0o644, func(w io.Writer) error {
		_, err := io.WriteString(w, "replacement")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replacement" {
		t.Fatalf("contents = %q, want replacement", got)
	}
}

func TestWriteFailurePreservesDestinationAndRemovesTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("encode failed")
	err := Write(path, 0o600, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("destination = %q, want original", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".state.toml-") {
			t.Fatalf("temporary file %q was not removed", entry.Name())
		}
	}
}
