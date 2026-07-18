package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestWriteSyncsEveryNewDirectoryEntry(t *testing.T) {
	root := t.TempDir()
	one := filepath.Join(root, "one")
	two := filepath.Join(one, "two")
	path := filepath.Join(two, "state.toml")
	var synced []string

	err := write(path, 0o600, func(w io.Writer) error {
		_, err := io.WriteString(w, "durable tree")
		return err
	}, func(dir string) error {
		synced = append(synced, dir)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{root, one, two}
	if fmt.Sprint(synced) != fmt.Sprint(want) {
		t.Fatalf("synced directories = %q, want %q", synced, want)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "durable tree" {
		t.Fatalf("destination = %q, %v", contents, err)
	}
}

func TestWriteReportsNewAncestorSyncFailure(t *testing.T) {
	wantErr := errors.New("ancestor sync failed")
	root := t.TempDir()
	path := filepath.Join(root, "one", "two", "state.toml")
	var synced []string
	err := write(path, 0o600, func(io.Writer) error {
		t.Fatal("encoder called after directory sync failed")
		return nil
	}, func(dir string) error {
		synced = append(synced, dir)
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
	if len(synced) != 1 || synced[0] != root {
		t.Fatalf("synced directories = %q, want only %q", synced, root)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("destination unexpectedly exists: %v", statErr)
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

func TestWriteNewNeverClobbersConcurrentCreator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	encoding := make(chan struct{})
	continueEncoding := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- WriteNew(path, 0o644, func(w io.Writer) error {
			if _, err := io.WriteString(w, "generated template"); err != nil {
				return err
			}
			close(encoding)
			<-continueEncoding
			return nil
		})
	}()
	<-encoding
	if err := os.WriteFile(path, []byte("user-created"), 0o600); err != nil {
		t.Fatal(err)
	}
	close(continueEncoding)
	if err := <-errCh; !errors.Is(err, fs.ErrExist) {
		t.Fatalf("WriteNew error = %v, want fs.ErrExist", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "user-created" {
		t.Fatalf("concurrent file was clobbered: %q", contents)
	}
}

func TestWriteNewHasExactlyOneConcurrentWinner(t *testing.T) {
	const writers = 16
	path := filepath.Join(t.TempDir(), "colors.conf")
	start := make(chan struct{})
	type result struct {
		content string
		err     error
	}
	results := make(chan result, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		content := fmt.Sprintf("template-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := WriteNew(path, 0o644, func(w io.Writer) error {
				_, err := io.WriteString(w, content)
				return err
			})
			results <- result{content: content, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winner := ""
	for result := range results {
		switch {
		case result.err == nil:
			if winner != "" {
				t.Fatalf("multiple creators won: %q and %q", winner, result.content)
			}
			winner = result.content
		case !errors.Is(result.err, fs.ErrExist):
			t.Fatalf("WriteNew error = %v, want fs.ErrExist", result.err)
		}
	}
	if winner == "" {
		t.Fatal("no concurrent creator won")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != winner {
		t.Fatalf("contents = %q, winning write = %q", contents, winner)
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
