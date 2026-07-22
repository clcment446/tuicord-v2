package ui

import (
	"awesomeProject/internal/tui/widget"
)

// queryList owns the repeated mechanics of a searchable picker: query state,
// filtered values, rows, and retained selection. Callers retain their own
// matching/ranking policy through the filter function.
type queryList[T any] struct {
	query    string
	list     *widget.ItemList
	filtered []T
	filter   func(string) ([]T, []widget.Item)
}

func newQueryList[T any](list *widget.ItemList, filter func(string) ([]T, []widget.Item)) *queryList[T] {
	q := &queryList[T]{list: list, filter: filter}
	q.Refilter()
	return q
}

func (q *queryList[T]) List() *widget.ItemList {
	if q == nil {
		return nil
	}
	return q.list
}

func (q *queryList[T]) Query() string {
	if q == nil {
		return ""
	}
	return q.query
}

func (q *queryList[T]) SetQuery(query string) {
	if q == nil {
		return
	}
	q.query = query
	q.Refilter()
}

func (q *queryList[T]) Refilter() {
	if q == nil || q.filter == nil || q.list == nil {
		return
	}
	selected := q.list.Selected()
	filtered, rows := q.filter(q.query)
	q.filtered = filtered
	q.list.SetItems(rows)
	q.list.SetSelectedSilent(selected)
}

func (q *queryList[T]) Selected() (T, bool) {
	var zero T
	if q == nil || q.list == nil {
		return zero, false
	}
	i := q.list.Selected()
	if i < 0 || i >= len(q.filtered) {
		return zero, false
	}
	return q.filtered[i], true
}
