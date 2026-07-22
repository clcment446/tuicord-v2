package ui

import (
	"testing"

	"awesomeProject/internal/tui/widget"
)

func TestQueryListFiltersAndPreservesSelection(t *testing.T) {
	items := []string{"alpha", "beta", "bravo"}
	list := newQueryList(widget.NewItemList(nil), func(query string) ([]string, []widget.Item) {
		var filtered []string
		var rows []widget.Item
		for _, item := range items {
			if query == "" || item[0:1] == query[0:1] {
				filtered = append(filtered, item)
				rows = append(rows, widget.Item{Label: item})
			}
		}
		return filtered, rows
	})
	list.SetQuery("b")
	list.List().SetSelectedSilent(1)
	list.Refilter()

	if got, ok := list.Selected(); !ok || got != "bravo" {
		t.Fatalf("selected = %q, %v; want bravo, true", got, ok)
	}
}
