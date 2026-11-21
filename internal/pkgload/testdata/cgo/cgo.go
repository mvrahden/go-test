// Package cgo has a cgo file in the package under test.
package cgo

/*
static int twice(int n) { return 2 * n; }
*/
import "C"

// Twice doubles n in C.
func Twice(n int) int { return int(C.twice(C.int(n))) }
