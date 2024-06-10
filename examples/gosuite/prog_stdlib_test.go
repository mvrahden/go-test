package gosuite

import "testing"

// Test_Unit_Stdlib is a simple stdlib-style test of the rolling list.
func Test_Unit_Stdlib(t *testing.T) {
	sut := NewUnit()
	for idx, expected := range []string{"hello", "world", "foo", "bar", "baz"} {
		actual := sut.DoSomething()
		if actual != expected {
			t.Fatalf("not equal@%d. wanted %q; got %q", idx, expected, actual)
		}
	}
}
