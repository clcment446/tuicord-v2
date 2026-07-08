package app

import (
	"encoding/json"
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
)

// Upstream arikawa drops min_values/max_values on unmarshal (ValueLimits is
// tagged `json:"-"` with only a MarshalJSON counterpart), which makes every
// incoming select look like a single select. The patched fork under
// third_party/arikawa restores them; this test fails if the go.mod replace
// directive or the patch is ever lost.
func TestIncomingSelectKeepsValueLimits(t *testing.T) {
	// Arrange
	payload := []byte(`{
		"type": 3,
		"custom_id": "sell_items",
		"placeholder": "Choose one or more items to sell",
		"min_values": 1,
		"max_values": 25,
		"options": [
			{"label": "Bronze Sword", "value": "101"},
			{"label": "Iron Helm", "value": "102"}
		]
	}`)

	// Act
	component, err := discord.ParseComponent(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("ParseComponent: %v", err)
	}
	node, ok := convertComponentNode(component)

	// Assert
	if !ok || node.Kind != store.ComponentSelect {
		t.Fatalf("converted node = %+v,%v, want select", node, ok)
	}
	if node.MinValues != 1 || node.MaxValues != 25 {
		t.Fatalf("value limits = [%d,%d], want [1,25] — is the third_party/arikawa patch still applied?",
			node.MinValues, node.MaxValues)
	}
}

func TestIncomingCheckboxGroupKeepsValueLimits(t *testing.T) {
	// Arrange
	payload := []byte(`{
		"type": 22,
		"custom_id": "perks",
		"min_values": 0,
		"max_values": 3,
		"options": [{"label": "Perk", "value": "p1"}]
	}`)

	// Act
	component, err := discord.ParseComponent(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("ParseComponent: %v", err)
	}
	node, ok := convertComponentNode(component)

	// Assert
	if !ok || node.Kind != store.ComponentCheckboxGroup {
		t.Fatalf("converted node = %+v,%v, want checkbox group", node, ok)
	}
	if node.MaxValues != 3 {
		t.Fatalf("max values = %d, want 3", node.MaxValues)
	}
}
