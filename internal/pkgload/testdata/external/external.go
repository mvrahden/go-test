// Package external has external tests only.
package external

import "fmt"

// Pair is two values.
type Pair struct{ A, B int }

// String renders p.
func (p Pair) String() string { return fmt.Sprintf("(%d, %d)", p.A, p.B) }
