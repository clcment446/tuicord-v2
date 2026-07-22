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

func TestRecordRecentStickerIsUniqueAndBounded(t *testing.T) {
	st := &State{}
	for id := uint64(1); id <= 25; id++ {
		st.RecordRecentSticker(id)
	}
	st.RecordRecentSticker(20)
	if len(st.RecentStickers) != 20 || st.RecentStickers[0] != 20 {
		t.Fatalf("recent stickers = %v", st.RecentStickers)
	}
	seen := map[uint64]bool{}
	for _, id := range st.RecentStickers {
		if seen[id] {
			t.Fatalf("duplicate recent sticker %d in %v", id, st.RecentStickers)
		}
		seen[id] = true
	}
}

func TestToggleFavorite(t *testing.T) {
	st := &State{}
	if !st.ToggleFavoriteEmoji("u:🔥") || !st.IsFavoriteEmoji("u:🔥") {
		t.Fatal("emoji favorite was not enabled")
	}
	if st.ToggleFavoriteEmoji("u:🔥") || st.IsFavoriteEmoji("u:🔥") {
		t.Fatal("emoji favorite was not disabled")
	}
	if !st.ToggleFavoriteSticker(42) || !st.IsFavoriteSticker(42) {
		t.Fatal("sticker favorite was not enabled")
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

func TestGuildLayoutCopiesAndReplaces(t *testing.T) {
	st := &State{}
	groups := []GuildGroup{{ID: -1, Name: "Work", GuildIDs: []uint64{10, 20}}}
	st.SetGuildLayout(7, groups)
	groups[0].GuildIDs[0] = 99

	got, ok := st.GuildLayout(7)
	if !ok || len(got) != 1 || got[0].GuildIDs[0] != 10 {
		t.Fatalf("GuildLayout = %+v,%v", got, ok)
	}
	got[0].GuildIDs[0] = 88
	again, _ := st.GuildLayout(7)
	if again[0].GuildIDs[0] != 10 {
		t.Fatalf("GuildLayout returned shared data: %+v", again)
	}

	st.SetGuildLayout(7, []GuildGroup{{ID: -2, Name: "Games", GuildIDs: []uint64{30}}})
	got, _ = st.GuildLayout(7)
	if len(st.GuildLayouts) != 1 || got[0].ID != -2 {
		t.Fatalf("SetGuildLayout did not replace account layout: %+v", st.GuildLayouts)
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
	st.SetGuildLayout(11, []GuildGroup{{ID: -1, Name: "Work", GuildIDs: []uint64{100, 200}}})
	st.Accounts = &Accounts{Active: 1, List: []Account{{Key: "token", Label: "Alice", ID: 11}, {Key: "acct-2", Label: "Bob", ID: 22}}}
	st.AuthPreferredMode = "browser"
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
	if got.ActiveAccount() != 1 || len(got.AccountList()) != 2 || got.AccountList()[1].Label != "Bob" || got.AuthPreferredMode != "browser" {
		t.Fatalf("machine startup state did not round-trip: %+v", got)
	}
	layout, ok := got.GuildLayout(11)
	if !ok || len(layout) != 1 || layout[0].Name != "Work" || len(layout[0].GuildIDs) != 2 {
		t.Fatalf("guild layout did not round-trip: %+v,%v", layout, ok)
	}
}

func TestSaveToAtomicallyReplacesExistingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.toml")
	if err := os.WriteFile(path, []byte("pinned_guilds = [1]"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := &State{PinnedGuilds: []uint64{2}}
	if err := want.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.PinnedGuilds) != 1 || got.PinnedGuilds[0] != 2 {
		t.Fatalf("PinnedGuilds = %v, want [2]", got.PinnedGuilds)
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
