package generic

// Index is a generic type with methods.
type Index[T any] struct{ items []T }

// Add appends v.
func (i *Index[T]) Add(v T) error { i.items = append(i.items, v); return nil }

// Len counts the items.
func (i *Index[T]) Len() int { return len(i.items) }

// Store is a generic interface.
type Store[K comparable] interface {
	Get(k K) (string, error)
}

// Map is a generic function.
func Map[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

// Ident is a generic function of one parameter.
func Ident[T any](v T) T { return v }

// Plain is a type with a method, beside the generic ones.
type Plain struct{}

// Do does nothing.
func (p *Plain) Do() error { return nil }
