// Package helpers fails an assertion inside a helper, so a failure names two
// lines: the helper's and its caller's.
package helpers

import "github.com/mvrahden/go-test/pkg/gotest"

type HelpersTestSuite struct{}

func (s *HelpersTestSuite) TestThroughAHelper(t *gotest.T) {
	expectAnswer(t, 41) // the caller's line
}

func expectAnswer(t *gotest.T, got int) {
	gotest.Equal(t, 42, got) // the helper's line
}
