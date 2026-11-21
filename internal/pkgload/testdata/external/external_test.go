package external_test

import (
	"testing"

	"github.com/mvrahden/go-test/internal/pkgload/testdata/external"
)

func TestString(t *testing.T) {
	if got := (external.Pair{A: 1, B: 2}).String(); got != "(1, 2)" {
		t.Fatal(got)
	}
}
