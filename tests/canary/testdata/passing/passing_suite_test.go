package passing_test

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PassingTestSuite is the canary's green control: every method must pass.
type PassingTestSuite struct{}

func (s *PassingTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, 1) }

func (s *PassingTestSuite) TestTwo(t *gotest.T) {
	t.When("a behavior is declared", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) { gotest.True(it, true) })
	})
}

type route string

// TestTypedArguments: assertions take named kinds and the edges of float64
// at face value.
func (s *PassingTestSuite) TestTypedArguments(t *gotest.T) {
	gotest.Regexp(t, route(`^/api/`), "/api/users")
	gotest.Contains(t, route("/api/users"), route("users"))
	gotest.InDelta(t, math.Inf(1), math.Inf(1), 0)
}

// TestJSONNumbers: an integer and its decimal spelling are the same JSON.
func (s *PassingTestSuite) TestJSONNumbers(t *gotest.T) { gotest.JSONEq(t, `{"n":1}`, `{"n":1.0}`) }

// TestEventuallyFinalPoll: a tick longer than waitFor still gets a last
// poll at the deadline.
func (s *PassingTestSuite) TestEventuallyFinalPoll(t *gotest.T) {
	var polls atomic.Int32
	gotest.Eventually(t, 200*time.Millisecond, 10*time.Second, func(poll *gotest.R) {
		gotest.Greater(poll, polls.Add(1), int32(1))
	})
}

// TestConsistentlyFinalPoll: Consistently, too, polls once more at the
// deadline when tick is longer than waitFor.
func (s *PassingTestSuite) TestConsistentlyFinalPoll(t *gotest.T) {
	var polls atomic.Int32
	gotest.Consistently(t, 200*time.Millisecond, 10*time.Second, func(*gotest.R) { polls.Add(1) })
	gotest.Equal(t, int32(2), polls.Load())
}

// TestDeadlineFollowsTheSuite: a tighter T derives from the suite's context,
// so its deadline is never later than the suite's own.
func (s *PassingTestSuite) TestDeadlineFollowsTheSuite(t *gotest.T) {
	d, ok := gotest.NewTWithDeadline(t, time.Hour).Context().Deadline()
	gotest.True(t, ok)
	gotest.Less(t, time.Until(d), time.Minute)
}
