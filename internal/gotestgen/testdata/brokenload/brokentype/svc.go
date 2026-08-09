// Package brokentype fails to type-check on purpose: the loader must book it
// as a broken package, never drop it.
package brokentype

func Answer() int {
	var s string = 42
	return s
}
