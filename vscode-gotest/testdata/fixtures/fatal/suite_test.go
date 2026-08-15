package fatal

import "github.com/mvrahden/go-test/pkg/gotest"

// t.FailNow unwinds through runtime.Goexit rather than returning, which is a
// different control path from an assertion failure and a different one again
// from a panic. Raw FailNow is the point of this fixture.
type FatalTestSuite struct{}

func (s *FatalTestSuite) TestAborts(t *gotest.T) {
	t.When("a precondition cannot be met", func(t *gotest.T) {
		t.It("aborts the behavior immediately", func(t *gotest.T) {
			t.T().Log("giving up before the assertion")
			t.FailNow()
		})
		t.It("does not stop the sibling behavior", func(t *gotest.T) {
			gotest.Equal(t, 1, 1)
		})
	})
}
