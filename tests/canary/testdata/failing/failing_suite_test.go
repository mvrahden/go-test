package failing_test

import (
	"errors"
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailingTestSuite fails once per assertion function. Every method must be
// reported as a failure; a family whose failure vanished is a broken kernel.
type FailingTestSuite struct{}

var errBoom = errors.New("boom")

func (s *FailingTestSuite) TestEqual(t *gotest.T)         { gotest.Equal(t, 1, 2) }
func (s *FailingTestSuite) TestNotEqual(t *gotest.T)      { gotest.NotEqual(t, 1, 1) }
func (s *FailingTestSuite) TestTrue(t *gotest.T)          { gotest.True(t, false) }
func (s *FailingTestSuite) TestFalse(t *gotest.T)         { gotest.False(t, true) }
func (s *FailingTestSuite) TestZero(t *gotest.T)          { gotest.Zero(t, 1) }
func (s *FailingTestSuite) TestNotZero(t *gotest.T)       { gotest.NotZero(t, 0) }
func (s *FailingTestSuite) TestNoError(t *gotest.T)       { gotest.NoError(t, errBoom) }
func (s *FailingTestSuite) TestError(t *gotest.T)         { gotest.Error(t, nil) }
func (s *FailingTestSuite) TestErrorIs(t *gotest.T)       { gotest.ErrorIs(t, errors.New("other"), errBoom) }
func (s *FailingTestSuite) TestErrorAs(t *gotest.T)       { gotest.ErrorAs[*os.PathError](t, errBoom) }
func (s *FailingTestSuite) TestErrorContains(t *gotest.T) { gotest.ErrorContains(t, errBoom, "zzz") }
func (s *FailingTestSuite) TestEmpty(t *gotest.T)         { gotest.Empty(t, []int{1}) }
func (s *FailingTestSuite) TestNotEmpty(t *gotest.T)      { gotest.NotEmpty(t, []int{}) }
func (s *FailingTestSuite) TestLen(t *gotest.T)           { gotest.Len(t, []int{1}, 2) }
func (s *FailingTestSuite) TestContains(t *gotest.T)      { gotest.Contains(t, "abc", "z") }
func (s *FailingTestSuite) TestNotContains(t *gotest.T)   { gotest.NotContains(t, "abc", "a") }
func (s *FailingTestSuite) TestElementsMatch(t *gotest.T) {
	gotest.ElementsMatch(t, []int{1}, []int{2})
}
func (s *FailingTestSuite) TestSubset(t *gotest.T)         { gotest.Subset(t, []int{1}, []int{2}) }
func (s *FailingTestSuite) TestGreater(t *gotest.T)        { gotest.Greater(t, 1, 2) }
func (s *FailingTestSuite) TestGreaterOrEqual(t *gotest.T) { gotest.GreaterOrEqual(t, 1, 2) }
func (s *FailingTestSuite) TestLess(t *gotest.T)           { gotest.Less(t, 2, 1) }
func (s *FailingTestSuite) TestLessOrEqual(t *gotest.T)    { gotest.LessOrEqual(t, 2, 1) }
func (s *FailingTestSuite) TestInDelta(t *gotest.T)        { gotest.InDelta(t, 1.0, 2.0, 0.1) }
func (s *FailingTestSuite) TestRegexp(t *gotest.T)         { gotest.Regexp(t, "^z", "abc") }
func (s *FailingTestSuite) TestJSONEq(t *gotest.T)         { gotest.JSONEq(t, `{"a":1}`, `{"a":2}`) }
func (s *FailingTestSuite) TestTimeWithin(t *gotest.T) {
	gotest.TimeWithin(t, time.Unix(0, 0), time.Unix(100, 0), time.Second)
}
func (s *FailingTestSuite) TestTimeIsNow(t *gotest.T) {
	gotest.TimeIsNow(t, time.Unix(0, 0), time.Second)
}
func (s *FailingTestSuite) TestPanics(t *gotest.T) { gotest.Panics(t, func() {}) }
func (s *FailingTestSuite) TestFail(t *gotest.T)   { gotest.Fail(t, "unconditional") }
func (s *FailingTestSuite) TestNil(t *gotest.T) {
	v := 1
	gotest.Nil(t, &v)
}
func (s *FailingTestSuite) TestNotNil(t *gotest.T) { gotest.NotNil(t, []int(nil)) }
func (s *FailingTestSuite) TestEventually(t *gotest.T) {
	gotest.Eventually(t, 60*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, false)
	})
}
func (s *FailingTestSuite) TestConsistently(t *gotest.T) {
	gotest.Consistently(t, 60*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, false)
	})
}
func (s *FailingTestSuite) TestBehavior(t *gotest.T) {
	t.When("a behavior fails", func(w *gotest.T) {
		w.It("is reported at the leaf", func(it *gotest.T) { gotest.Equal(it, "a", "b") })
	})
}
