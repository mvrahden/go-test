// Package crossdep supplies a fuzzable type that lives in a DIFFERENT
// package from the suite that fuzzes it — the shape every external (pxtest)
// fuzz target has, where the type under test is never in the test package.
package crossdep

type Setting struct {
	Key   string
	Value int
}

// ID is a named basic type — the shape that must render as a QUALIFIED
// explicit conversion (crossdep.ID(...)) in a literal, never the bare
// identifier (ID(...)), which is out of scope outside this package.
type ID string
