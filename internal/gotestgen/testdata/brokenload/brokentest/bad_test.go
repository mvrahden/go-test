// The type error lives only in the test file; the package must still be a
// broken package, because its test build fails.
package brokentest

import "testing"

func TestThree(t *testing.T) {
	var s string = Three()
	_ = s
}
