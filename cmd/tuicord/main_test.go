package main

import (
	"testing"

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
