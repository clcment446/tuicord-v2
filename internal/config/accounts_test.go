package config

import "testing"

// TestAccountsRoundTrip guards the multi-account registry serialization: the
// pointer-held Accounts section (kept a pointer so Config stays comparable) must
// survive a Save/Load cycle, and the reduced default panel widths must hold.
func TestAccountsRoundTrip(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	cfg := Default()
	cfg.Accounts = &Accounts{Active: 1, List: []Account{
		{Key: "token", Label: "Alice", ID: 111},
		{Key: "acct-2", Label: "Bob", ID: 222},
	}}
	if err := saveTo(path, cfg); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if got.ActiveAccount() != 1 {
		t.Fatalf("active = %d, want 1", got.ActiveAccount())
	}
	list := got.AccountList()
	if len(list) != 2 || list[0].Key != "token" || list[1].Label != "Bob" || list[1].ID != 222 {
		t.Fatalf("round-trip mismatch: %+v", list)
	}
	if got.Layout.GuildsWidth != 3 || got.Layout.ChannelsWidth != 20 {
		t.Fatalf("default widths = %d/%d, want 3/20", got.Layout.GuildsWidth, got.Layout.ChannelsWidth)
	}
}

// TestDefaultConfigStaysComparable protects the invariant that Config is
// comparable (Default() != Default() would fail at compile time if a bare slice
// or map field were added).
func TestDefaultConfigStaysComparable(t *testing.T) {
	if Default() != Default() {
		t.Fatal("Default() values are not equal; Config lost comparability")
	}
}
