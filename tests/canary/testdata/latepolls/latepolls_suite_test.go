package latepolls_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LatePollsTestSuite: a poll that started before waitFor and returns after
// it, within the next tick, is judged by its result.
type LatePollsTestSuite struct{}

// slowPass passes after 700ms: started before a 500ms waitFor, it returns
// after it and within the 500ms tick that follows.
func slowPass(*gotest.R) { time.Sleep(700 * time.Millisecond) }

func (s *LatePollsTestSuite) TestConsistentlyPassesALatePoll(t *gotest.T) {
	gotest.Consistently(t, 500*time.Millisecond, 500*time.Millisecond, slowPass)
}

func (s *LatePollsTestSuite) TestEventuallyPassesALatePoll(t *gotest.T) {
	gotest.Eventually(t, 500*time.Millisecond, 500*time.Millisecond, slowPass)
}
