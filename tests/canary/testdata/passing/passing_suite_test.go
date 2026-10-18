package passing_test

import (
	"math"

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
