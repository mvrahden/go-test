package search

import "strings"

type Indexable interface {
	comparable
	SearchText() string
	Label() string
}

type Article struct {
	Title string
	Body  string
}

func (a Article) SearchText() string { return a.Title + " " + a.Body }
func (a Article) Label() string      { return a.Title }

type Product struct {
	Name        string
	Description string
}

func (p Product) SearchText() string { return p.Name + " " + p.Description }
func (p Product) Label() string      { return p.Name }

type index[T Indexable] struct {
	items []T
}

func newIndex[T Indexable](items ...T) *index[T] {
	return &index[T]{items: items}
}

func (idx *index[T]) Search(query string) []T {
	q := strings.ToLower(query)
	var results []T
	for _, it := range idx.items {
		if strings.Contains(strings.ToLower(it.SearchText()), q) {
			results = append(results, it)
		}
	}
	return results
}

func (idx *index[T]) Labels() []string {
	ls := make([]string, len(idx.items))
	for i, it := range idx.items {
		ls[i] = it.Label()
	}
	return ls
}

func (idx *index[T]) All() []T {
	out := make([]T, len(idx.items))
	copy(out, idx.items)
	return out
}
