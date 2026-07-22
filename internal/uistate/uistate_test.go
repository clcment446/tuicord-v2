package uistate

import (
	"os"
	"path/filepath"
	"strings"
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
	st.ToggleCollapsedFolder("acct-a", 7, -1)
	st.ToggleCollapsedCategory(8)
	if !st.IsFolderCollapsed("acct-a", 7, -1) || !st.IsCategoryCollapsed(8) {
		t.Fatal("collapse toggles not recorded")
	}
	set := st.CollapsedFolderSet("acct-a", 7)
	if !set[-1] || len(set) != 1 {
		t.Fatalf("CollapsedFolderSet = %v, want {-1:true}", set)
	}
}

func TestCollapsedFoldersAreAccountScoped(t *testing.T) {
	st := &State{}
	st.ToggleCollapsedFolder("acct-a", 0, -1)
	if !st.IsFolderCollapsed("acct-a", 0, -1) {
		t.Fatal("first account did not retain its local folder collapse")
	}
	if st.IsFolderCollapsed("acct-b", 0, -1) {
		t.Fatal("same local folder ID leaked into second account")
	}
	st.ToggleCollapsedFolder("acct-b", 0, -1)
	st.ToggleCollapsedFolder("acct-a", 0, -1)
	if st.IsFolderCollapsed("acct-a", 0, -1) || !st.IsFolderCollapsed("acct-b", 0, -1) {
		t.Fatalf("account collapse sets collided: %+v", st.GuildLayouts)
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
	if st.IsFolderCollapsed("acct", 7, -1) {
		t.Fatal("empty state should have no collapsed folders")
	}
	if !st.ToggleCollapsedFolder("acct", 7, -1) {
		t.Fatal("first toggle should collapse")
	}
	if st.ToggleCollapsedFolder("acct", 7, -1) {
		t.Fatal("second toggle should expand")
	}
	if st.IsFolderCollapsed("acct", 7, -1) {
		t.Fatal("folder should be expanded after second toggle")
	}
}

func TestGuildLayoutCopiesAndReplaces(t *testing.T) {
	st := &State{}
	groups := []GuildGroup{{ID: -1, Name: "Work", GuildIDs: []uint64{10, 20}}}
	st.SetGuildLayout("acct", 7, groups)
	groups[0].GuildIDs[0] = 99

	got, ok := st.GuildLayout("acct", 7)
	if !ok || len(got) != 1 || got[0].GuildIDs[0] != 10 {
		t.Fatalf("GuildLayout = %+v,%v", got, ok)
	}
	got[0].GuildIDs[0] = 88
	again, _ := st.GuildLayout("acct", 7)
	if again[0].GuildIDs[0] != 10 {
		t.Fatalf("GuildLayout returned shared data: %+v", again)
	}

	st.SetGuildLayout("acct", 7, []GuildGroup{{ID: -2, Name: "Games", GuildIDs: []uint64{30}}})
	got, _ = st.GuildLayout("acct", 7)
	if len(st.GuildLayouts) != 1 || got[0].ID != -2 {
		t.Fatalf("SetGuildLayout did not replace account layout: %+v", st.GuildLayouts)
	}
}

func TestGuildLayoutsUseKeysBeforeAccountHydration(t *testing.T) {
	st := &State{}
	st.SetGuildLayout("acct-a", 0, []GuildGroup{{ID: -1, Name: "A"}})
	st.SetGuildLayout("acct-b", 0, []GuildGroup{{ID: -1, Name: "B"}})
	if len(st.GuildLayouts) != 2 {
		t.Fatalf("zero-ID layouts collided: %+v", st.GuildLayouts)
	}
	path := filepath.Join(t.TempDir(), "ui.toml")
	if err := st.saveTo(path); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	first, okA := st.GuildLayout("acct-a", 0)
	second, okB := st.GuildLayout("acct-b", 0)
	if !okA || !okB || first[0].Name != "A" || second[0].Name != "B" {
		t.Fatalf("keyed layouts = %+v / %+v", first, second)
	}
	if hydrated, ok := st.GuildLayout("acct-a", 77); !ok || hydrated[0].Name != "A" || st.GuildLayouts[0].AccountID != 77 {
		t.Fatalf("layout disappeared after hydration: %+v,%v layouts=%+v", hydrated, ok, st.GuildLayouts)
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
	st.Accounts = &Accounts{Active: 1, List: []Account{{Key: "token", Label: "Alice", ID: 11}, {Key: "acct-2", Label: "Bob", ID: 22}}}
	st.TogglePinnedGuild(100)
	st.TogglePinnedChannel(200)
	st.ToggleCollapsedFolder("token", 11, -3)
	st.ToggleCollapsedCategory(300)
	st.SetGuildLayout("token", 11, []GuildGroup{{ID: -1, Name: "Work", GuildIDs: []uint64{100, 200}}})
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
		!got.IsFolderCollapsed("token", 11, -3) || !got.IsCategoryCollapsed(300) {
		t.Fatalf("round-trip lost state: %+v", got)
	}
	if got.ActiveAccount() != 1 || len(got.AccountList()) != 2 || got.AccountList()[1].Label != "Bob" || got.AuthPreferredMode != "browser" {
		t.Fatalf("machine startup state did not round-trip: %+v", got)
	}
	layout, ok := got.GuildLayout("token", 11)
	if !ok || len(layout) != 1 || layout[0].Name != "Work" || len(layout[0].GuildIDs) != 2 {
		t.Fatalf("guild layout did not round-trip: %+v,%v", layout, ok)
	}
}

func TestLoadMigratesLegacyGuildLayoutState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.toml")
	legacy := `collapsed_folders = [-1, -2]

[[guild_layouts]]
account_id = 42

[[guild_layouts.groups]]
id = -1
name = "Work"
guild_ids = [100]

[accounts]
active = 0

[[accounts.list]]
key = "acct-a"
label = "Alice"
id = 42
`
	if err := writeFile(t, path, legacy); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := st.GuildLayout("acct-a", 42)
	if !ok || len(groups) != 1 || groups[0].Name != "Work" {
		t.Fatalf("legacy ID layout not found by key: %+v,%v", groups, ok)
	}
	if !st.IsFolderCollapsed("acct-a", 42, -1) || !st.IsFolderCollapsed("acct-a", 42, -2) {
		t.Fatalf("legacy collapsed folders not migrated: %+v", st.GuildLayouts)
	}
	if len(st.CollapsedFolders) != 0 || st.GuildLayouts[0].AccountKey != "acct-a" {
		t.Fatalf("legacy fields were not normalized: %+v", st)
	}
	if err := st.saveTo(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	layoutsAt := strings.Index(text, "[[guild_layouts]]")
	if layoutsAt < 0 || strings.Contains(text[:layoutsAt], "collapsed_folders") {
		t.Fatalf("save retained global collapsed_folders:\n%s", text)
	}
	if !strings.Contains(text, `account_key = "acct-a"`) {
		t.Fatalf("save did not key-associate legacy layout:\n%s", text)
	}
}

func TestLegacyCollapseMigratesAfterAccountsAreSeeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.toml")
	if err := writeFile(t, path, "collapsed_folders = [-1]\n"); err != nil {
		t.Fatal(err)
	}
	st, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Accounts = &Accounts{List: []Account{{Key: "acct-a", ID: 42}}}
	if err := st.saveTo(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.IsFolderCollapsed("acct-a", 42, -1) {
		t.Fatalf("late-seeded account lost legacy collapse: %+v", reloaded.GuildLayouts)
	}
}

func TestIDOnlyGuildLayoutAssociatesWhenUsed(t *testing.T) {
	st := &State{GuildLayouts: []GuildLayout{{AccountID: 99, Groups: []GuildGroup{{Name: "Old"}}}}}
	groups, ok := st.GuildLayout("acct-old", 99)
	if !ok || groups[0].Name != "Old" || st.GuildLayouts[0].AccountKey != "acct-old" {
		t.Fatalf("ID-only layout was not adopted: groups=%+v layouts=%+v", groups, st.GuildLayouts)
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
