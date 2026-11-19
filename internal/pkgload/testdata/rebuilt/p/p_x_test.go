package p_test

import (
	"testing"

	"github.com/mvrahden/go-test/internal/pkgload/testdata/rebuilt/q"
)

var made = q.Make("x")

func TestMake(t *testing.T) {
	if made.Name != "x" {
		t.Fatal()
	}
}
