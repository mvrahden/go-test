package panichook

import "github.com/mvrahden/go-test/pkg/gotest"

// A panic in a lifecycle hook, not in a behavior. The failure belongs to the
// setup, and every behavior underneath must still be accounted for rather than
// silently vanishing from the tree.
type PanicHookTestSuite struct{}

func (s *PanicHookTestSuite) BeforeEach(t *gotest.T) {
	panic("deliberate panic in BeforeEach")
}

func (s *PanicHookTestSuite) TestNeverRuns(t *gotest.T) {
	t.When("setup panicked", func(t *gotest.T) {
		t.It("cannot report a passing behavior", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
