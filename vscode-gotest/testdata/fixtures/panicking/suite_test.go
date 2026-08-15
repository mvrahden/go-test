package panicking

import "github.com/mvrahden/go-test/pkg/gotest"

// A panic inside a behavior must be contained: the suite reports a failure and
// the rest of the run keeps going, rather than taking the process down.
type PanickingTestSuite struct{}

func (s *PanickingTestSuite) TestExplodes(t *gotest.T) {
	t.When("the code under test panics", func(t *gotest.T) {
		t.It("is contained as a failure", func(t *gotest.T) {
			panic("deliberate panic from fixture")
		})
		t.It("does not prevent the next behavior", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
