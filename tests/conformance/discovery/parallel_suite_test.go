package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ParallelShapesTestSuite: what a suite with Parallel: true cannot do safely
// is refused where it is declared; the shapes that cannot go wrong run.
type ParallelShapesTestSuite struct{}

func (s *ParallelShapesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ParallelShapesTestSuite) TestFixtureHooks(t *gotest.T) {
	t.When("a suite with Parallel: true binds a package fixture that declares BeforeEach or AfterEach", func(w *gotest.T) {
		w.It("a suite with two or more test methods whose fixture hook uses the fixture is refused at the suite, told to move the work into its returning BeforeEach or drop Parallel", func(it *gotest.T) {
			checkShape(it, shape{pkg: "parallelhooks", exit: 2,
				shows: `parallelhooks_suite_test\.go:23:6: suite RacingTestSuite has Parallel: true, but BeforeEach of fixture TableFixture uses the fixture, which the suite's parallel methods share: the hook would run for all of them at once — do the per-method work in the suite's returning BeforeEach, or drop Parallel`})
		})
		w.It("a suite whose fixture hooks never use the fixture runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "parallelhooksok", verdict: "TestStubHooksTestSuite/TestA"})
		})
		w.It("a suite with one test method runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "parallelhooksok", verdict: "TestSingleTestSuite/TestOnly"})
		})
	})
}
