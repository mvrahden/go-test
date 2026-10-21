// Package modhelpers runs a behavior another module declares, which fails
// inside that module's helper.
package modhelpers

import (
	"plaincheck/check"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ModHelpersTestSuite struct{}

func (s *ModHelpersTestSuite) TestThroughAnotherModule(t *gotest.T) {
	check.Answers(t)
}
