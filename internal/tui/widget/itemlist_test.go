package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
)

func TestItemListMousePressSelectsRow(t *testing.T) {
	list := NewItemList([]Item{
		{Label: "guild one"},
		{Label: "guild two"},
		{Label: "guild three"},
	})
	var selected []int
	list.OnSelect(func(index int) {
		selected = append(selected, index)
	})

	if !list.Handle(input.MouseEvent{Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("mouse press was not handled")
	}
	if got := list.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
	if len(selected) != 1 || selected[0] != 1 {
		t.Fatalf("onSelect calls = %v, want [1]", selected)
	}

	if !list.Handle(input.MouseEvent{Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("repeat mouse press was not handled")
	}
	if len(selected) != 2 || selected[1] != 1 {
		t.Fatalf("repeat onSelect calls = %v, want second call for row 1", selected)
	}
}

func TestItemListSetSelectedSilentDoesNotNotify(t *testing.T) {
	list := NewItemList([]Item{
		{Label: "one"},
		{Label: "two"},
	})
	var selected []int
	list.OnSelect(func(index int) {
		selected = append(selected, index)
	})

	list.SetSelectedSilent(1)

	if got := list.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
	if len(selected) != 0 {
		t.Fatalf("onSelect calls = %v, want none", selected)
	}
}
