// Package checks is a helper package of the user's in a directory named
// like gotest's own.
package checks

import "github.com/mvrahden/go-test/pkg/gotest"

// Positive fails unless n is above zero.
func Positive(t *gotest.T, n int) {
	gotest.Greater(t, n, 0) // the helper's line
}
