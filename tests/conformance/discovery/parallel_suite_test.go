package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
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

func (s *ParallelShapesTestSuite) TestSetenv(t *gotest.T) {
	refused, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/parallelsetenv/")
	gotest.NoError(t, err)
	out := refused.Stdout + refused.Stderr
	legal, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/parallelsetenvok/")
	gotest.NoError(t, err)

	t.When("a suite with Parallel: true calls Setenv on a T", func(w *gotest.T) {
		w.It("a call in a test method, in BeforeEach, in a nested It and through t.T() is refused at the call, told to set it in BeforeAll or drop Parallel", func(it *gotest.T) {
			gotest.Equal(it, 2, refused.Exit, "output: %s", out)
			for _, row := range []struct{ at, method string }{
				{"13:4", "MethodEnvTestSuite.TestSets"},
				{"26:4", "HookEnvTestSuite.BeforeEach"},
				{"41:6", "NestedEnvTestSuite.TestSets"},
				{"53:8", "StdlibEnvTestSuite.TestSets"},
			} {
				gotest.Contains(it, out, "parallelsetenv_suite_test.go:"+row.at+": "+row.method+" calls Setenv, but the suite has Parallel: true and testing panics when a parallel test sets the environment; set it in BeforeAll, or drop Parallel")
			}
		})
		w.It("a call in BeforeAll runs and its value reaches every method", func(it *gotest.T) {
			gotest.Equal(it, 0, legal.Exit, "output: %s", legal.Stdout+legal.Stderr)
			gotest.Equal(it, "pass", legal.Verdict("TestSetupEnvTestSuite/TestReadsA"), "output: %s", legal.Stdout)
			gotest.Equal(it, "pass", legal.Verdict("TestSetupEnvTestSuite/TestReadsB"), "output: %s", legal.Stdout)
		})
	})
}
