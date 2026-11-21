// Package both has in-package and external tests.
package both

import "sort"

// Sorted returns a sorted copy of xs.
func Sorted(xs []int) []int {
	out := append([]int(nil), xs...)
	sort.Ints(out)
	return out
}

func first(xs []int) int { return xs[0] }
