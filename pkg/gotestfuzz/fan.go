package gotestfuzz

import "testing"

// Adapter is what the generated fuzz wrapper hands to gotest.NewF: the
// fan for one fuzz-adapter instantiation in the package. It is sealed — the
// unexported method keeps every implementation in this package, so the
// mechanism can be replaced without a deprecation cycle. gotest.Fuzz*
// selects the fan for its own type arguments by type assertion.
type Adapter interface {
	adapter()
}

// Fan carries a single-argument fuzz target whose argument type A the
// engine cannot mutate well natively — a struct, a named type, or a plain
// number. Register calls (*testing.F).Fuzz directly with A's leaves as
// separate typed arguments and fans them back into an A per execution;
// Explode turns a typed seed into the same leaves for f.Add; Literal
// renders an A as self-contained Go source for the failure-time echo.
//
// You never construct one. Its fields are exported only so generated code
// can build it as a composite literal.
type Fan[A any] struct {
	Register func(f *testing.F, run func(*testing.T, A))
	Explode  func(A) []any
	Literal  func(A) string
}

// Fan2 is Fan for a two-argument target; each position that is not
// a pass-through kind fans independently, and pass-through positions stay
// exactly as declared.
type Fan2[A, B any] struct {
	Register func(f *testing.F, run func(*testing.T, A, B))
	Explode  func(A, B) []any
	Literal  func(A, B) string
}

// Fan3 is Fan for a three-argument target.
type Fan3[A, B, C any] struct {
	Register func(f *testing.F, run func(*testing.T, A, B, C))
	Explode  func(A, B, C) []any
	Literal  func(A, B, C) string
}

func (Fan[A]) adapter()        {}
func (Fan2[A, B]) adapter()    {}
func (Fan3[A, B, C]) adapter() {}
