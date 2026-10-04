package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// A panic leaving an It or When body is on its way to the testing package,
// which ends the process after the cleanups; fixture teardown reads the mark
// in those cleanups. A panic the user recovers must leave it unset.
//
// Sequential: the mark is process-wide.
type DyingMarkTestSuite struct{}

func (s *DyingMarkTestSuite) BeforeEach(t *gotest.T) { gotest.ExportResetDying() }
func (s *DyingMarkTestSuite) AfterEach(t *gotest.T)  { gotest.ExportResetDying() }

func (s *DyingMarkTestSuite) TestAPanickingBody(t *gotest.T) {
	t.When("an It body panics", func(w *gotest.T) {
		w.It("marks the process as dying", func(it *gotest.T) {
			gotest.Panics(it, func() {
				gotest.ExportExecTestFn(func(*gotest.T) { panic("boom") }, it)
			})
			gotest.True(it, gotest.ExportDying())
		})
	})
}

func (s *DyingMarkTestSuite) TestARecoveredPanic(t *gotest.T) {
	t.When("the user recovers a panic with Panics", func(w *gotest.T) {
		w.It("leaves the mark unset", func(it *gotest.T) {
			gotest.Panics(it, func() { panic("expected") })
			gotest.False(it, gotest.ExportDying())
		})
	})
}
