package failing_test

import (
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailingTestSuite fails once per assertion family. Every method must be
// reported as a failure; a family whose failure vanished is a broken kernel.
type FailingTestSuite struct{}

func (s *FailingTestSuite) TestEqual(t *gotest.T)    { gotest.Equal(t, 1, 2) }
func (s *FailingTestSuite) TestNoError(t *gotest.T)  { gotest.NoError(t, errors.New("boom")) }
func (s *FailingTestSuite) TestContains(t *gotest.T) { gotest.Contains(t, "abc", "z") }
func (s *FailingTestSuite) TestTrue(t *gotest.T)     { gotest.True(t, false) }
func (s *FailingTestSuite) TestNil(t *gotest.T) {
	v := 1
	gotest.Nil(t, &v)
}
func (s *FailingTestSuite) TestLen(t *gotest.T) { gotest.Len(t, []int{1}, 2) }
func (s *FailingTestSuite) TestEventually(t *gotest.T) {
	gotest.Eventually(t, 60*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, false)
	})
}
func (s *FailingTestSuite) TestBehavior(t *gotest.T) {
	t.When("a behavior fails", func(w *gotest.T) {
		w.It("is reported at the leaf", func(it *gotest.T) { gotest.Equal(it, "a", "b") })
	})
}
