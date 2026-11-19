package both_test

import (
	"testing"

	"github.com/mvrahden/go-test/internal/pkgload/testdata/both"
)

func TestSmallest(t *testing.T) {
	if both.Smallest([]int{3, 1, 2}) != 1 {
		t.Fatal()
	}
}
