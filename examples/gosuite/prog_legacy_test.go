package gosuite

import "testing"

// Test_Unit_Legacy is a simple test of the rolling list index.
func Test_Unit_Legacy(t *testing.T) {
	sut := NewUnit()
	for idx, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		actual := sut.DoSomething()
		if actual != expected {
			t.Fatalf("not equal@%d. wanted %q; got %q", idx, expected, actual)
		}
	}
}
