// Package check declares a behavior in a module whose path has no dot.
package check

import "github.com/mvrahden/go-test/pkg/gotest"

// Answers declares a behavior that fails through a helper.
func Answers(t *gotest.T) {
	t.It("answers", func(it *gotest.T) {
		expectAnswer(it, 41) // the caller's line
	})
}

func expectAnswer(t *gotest.T, got int) {
	gotest.Equal(t, 42, got) // the helper's line
}
