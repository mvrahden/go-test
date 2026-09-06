package passing_test

import "github.com/mvrahden/go-test/pkg/gotest"

// PassingTestSuite is the canary's green control: every method must pass.
type PassingTestSuite struct{}

func (s *PassingTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, 1) }

func (s *PassingTestSuite) TestTwo(t *gotest.T) {
	t.When("a behavior is declared", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) { gotest.True(it, true) })
	})
}
