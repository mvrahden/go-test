package both

import "testing"

// Smallest is exported to the external test.
func Smallest(xs []int) int { return first(Sorted(xs)) }

func TestFirst(t *testing.T) {
	if first([]int{3}) != 3 {
		t.Fatal()
	}
}
