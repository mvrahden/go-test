package stdlib_test

import (
	"testing"

	"github.com/mvrahden/go-test/examples/stdlib"
)

// Test_Unit_Stdlib is a simple stdlib-style test of the Unit.
func Test_Unit_Stdlib(t *testing.T) {
	sut := stdlib.NewUnit()
	for idx, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		actual := sut.DoSomething()
		if actual != expected {
			t.Fatalf("not equal@%d. wanted %q; got %q", idx, expected, actual)
		}
	}
}
