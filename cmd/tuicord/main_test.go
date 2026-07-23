package main

import (
	"testing"

	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/widget"
	"awesomeProject/internal/uistate"
)

func TestUIStylesUnknownBorderStyleFallsBackToRounded(t *testing.T) {
	cfg := config.Default()
	cfg.Display.BorderStyle = "unknown"

	if got := uiStyles(cfg).BorderChars; got != widget.BorderCharsForStyle("rounded") {
		t.Fatalf("unknown border style = %+v, want rounded", got)
	}
}

func TestDedupAccountRegistryDropsRepeatedKeys(t *testing.T) {
	list := []uistate.Account{
		{Key: "alice", Label: "Alice"},
		{Key: "bob", Label: "Bob"},
		{Key: "alice", Label: "Alice again"}, // same token as row 0
		{Key: "", Label: "Legacy A"},
		{Key: "", Label: "Legacy B"}, // empty keys are never merged
	}

	out, active := dedupAccountRegistry(list, "bob")

	if len(out) != 4 {
		t.Fatalf("deduped length = %d, want 4 (%v)", len(out), out)
	}
	if out[0].Key != "alice" || out[1].Key != "bob" || out[2].Key != "" || out[3].Key != "" {
		t.Fatalf("deduped keys = %+v", out)
	}
	if out[0].Label != "Alice" {
		t.Fatalf("kept the later duplicate: %q", out[0].Label)
	}
	if active != 1 {
		t.Fatalf("active index = %d, want 1 (bob)", active)
	}
}

func TestDedupAccountRegistryReindexesActivePastDuplicate(t *testing.T) {
	list := []uistate.Account{
		{Key: "dup"},
		{Key: "dup"}, // duplicate before the active row shifts its index
		{Key: "target"},
	}

	out, active := dedupAccountRegistry(list, "target")

	if len(out) != 2 || out[1].Key != "target" {
		t.Fatalf("deduped = %+v", out)
	}
	if active != 1 {
		t.Fatalf("active index = %d, want 1", active)
	}
}

func TestNewAccountKeyAndLabel(t *testing.T) {
	// Matrix credentials are keyed and labeled by user ID (stable across re-login).
	creds := auth.Credentials{Protocol: auth.ProtocolMatrix, UserID: "@alice:example.org"}
	value, err := creds.Encode()
	if err != nil {
		t.Fatal(err)
	}
	key, err := newAccountKey(value)
	if err != nil {
		t.Fatal(err)
	}
	if want := "matrix:@alice:example.org"; key != want {
		t.Fatalf("matrix key = %q, want %q", key, want)
	}
	if got := defaultAccountLabel(value); got != "@alice:example.org" {
		t.Fatalf("matrix label = %q, want the user ID", got)
	}

	// Discord is a bare token: random, unique keys and a placeholder label.
	k1, err := newAccountKey("some.discord.token")
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := newAccountKey("some.discord.token")
	if k1 == k2 {
		t.Fatal("expected distinct random keys for Discord accounts")
	}
	for _, k := range []string{k1, k2} {
		if len(k) <= len("discord:") || k[:len("discord:")] != "discord:" {
			t.Fatalf("discord key %q missing discord: prefix", k)
		}
	}
	if got := defaultAccountLabel("some.discord.token"); got != "New account" {
		t.Fatalf("discord label = %q, want placeholder", got)
	}
}
