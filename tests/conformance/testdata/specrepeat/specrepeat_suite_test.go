// Package specrepeat fails one method in the second of its runs; see the
// spec conformance suite.
package specrepeat

import "github.com/mvrahden/go-test/pkg/gotest"

// runs counts the suite's runs in this process, as -count repeats them.
var runs int

type RepeatTestSuite struct{}

func (s *RepeatTestSuite) BeforeAll(t *gotest.T) { runs++ }

func (s *RepeatTestSuite) AfterAll(t *gotest.T) {}

func (s *RepeatTestSuite) TestFailsOnItsSecondRun(t *gotest.T) {
	if runs == 2 {
		gotest.Fail(t, "second run")
	}
}

func (s *RepeatTestSuite) TestAlwaysPasses(t *gotest.T) {}
