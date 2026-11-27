package shop

import "testing"

//nolint:stdlib-test // a plain stdlib test
func TestPlain(t *testing.T) {
	if Add(0, 1) != 1 {
		t.Fatal("0 + 1")
	}
}
