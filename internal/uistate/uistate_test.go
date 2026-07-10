package uistate

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestToggleAndQuery(t *testing.T) {
	st := &State{}
	if st.IsPinnedChannel(42) {
		t.Fatal("empty state should have nothing pinned")
	}
	if got := st.TogglePinnedChannel(42); !got {
		t.Fatalf("first toggle should pin, got %v", got)
	}
	if !st.IsPinnedChannel(42) {
		t.Fatal("channel should be pinned after toggle")
	}
	if got := st.TogglePinnedChannel(42); got {
		t.Fatalf("second toggle should unpin, got %v", got)
	}
	if st.IsPinnedChannel(42) {
		t.Fatal("channel should be unpinned after second toggle")
	}
}

func TestToggleCollapsed(t *testing.T) {
	st := &State{}
	st.ToggleCollapsedFolder(7)
	st.ToggleCollapsedCategory(8)
	if !st.IsFolderCollapsed(7) || !st.IsCategoryCollapsed(8) {
		t.Fatal("collapse toggles not recorded")
	}
	set := st.CollapsedFolderSet()
	if !set[7] || len(set) != 1 {
		t.Fatalf("CollapsedFolderSet = %v, want {7:true}", set)
	}
}

func TestToggleCollapsedFolderOffAndQuery(t *testing.T) {
	st := &State{}
	if st.IsFolderCollapsed(7) {
		t.Fatal("empty state should have no collapsed folders")
	}
	if !st.ToggleCollapsedFolder(7) {
		t.Fatal("first toggle should collapse")
	}
	if st.ToggleCollapsedFolder(7) {
		t.Fatal("second toggle should expand")
	}
	if st.IsFolderCollapsed(7) {
		t.Fatal("folder should be expanded after second toggle")
	}
}

func TestPathHomeFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(home, ".local", "state", AppName, "ui.toml")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	st, err := Load()
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	st.TogglePinnedGuild(100)
	st.TogglePinnedChannel(200)
	st.ToggleCollapsedFolder(-3)
	st.ToggleCollapsedCategory(300)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	want := filepath.Join(dir, AppName, "ui.toml")
	if p, _ := Path(); p != want {
		t.Fatalf("Path = %q, want %q", p, want)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.IsPinnedGuild(100) || !got.IsPinnedChannel(200) ||
		!got.IsFolderCollapsed(-3) || !got.IsCategoryCollapsed(300) {
		t.Fatalf("round-trip lost state: %+v", got)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	st, err := Load()
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(st.PinnedGuilds) != 0 || len(st.CollapsedCategories) != 0 {
		t.Fatalf("missing file should yield empty state, got %+v", st)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	path, _ := Path()
	if err := writeFile(t, path, "this is = = not toml"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected decode error for corrupt file")
	}
}
