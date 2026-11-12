package discovery_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// NeverRunTestSuite: what reads as a case and no run would reach is refused
// where it is written; a helper something calls and a library's type are not.
type NeverRunTestSuite struct{}

func (s *NeverRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const orphanTypeRefusal = `UserSuite.TestCreate takes *gotest.T, but UserSuite is not a test suite (its name must end in TestSuite): the test never runs, nor do 2 more of its methods`

func (s *NeverRunTestSuite) TestCaseNamedMethods(t *gotest.T) {
	refused, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/orphantype/")
	gotest.NoError(t, err)
	helper, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/orphanhelper/")
	gotest.NoError(t, err)
	helperOut := helper.Stdout + helper.Stderr

	t.When("a test file declares methods named as cases, taking a gotest T, B or F, on a type that is no suite", func(w *gotest.T) {
		w.It("a type nothing calls them on is refused at its first such method, told which suffix makes it a suite", func(it *gotest.T) {
			out := refused.Stdout + refused.Stderr
			gotest.Equal(it, 2, refused.Exit, "output: %s", out)
			gotest.Contains(it, out, "orphantype_suite_test.go:8:21: "+orphanTypeRefusal)
			gotest.Equal(it, 1, strings.Count(out, "is not a test suite"), "output: %s", out)
		})
		w.It("a helper whose method a suite calls, directly or through an interface, runs", func(it *gotest.T) {
			gotest.Equal(it, 0, helper.Exit, "output: %s", helperOut)
			gotest.Equal(it, "pass", helper.Verdict("TestHelperTestSuite/TestCallsHelper"), "output: %s", helperOut)
			gotest.Equal(it, "pass", helper.Verdict("TestHelperTestSuite/TestCallsThroughInterface"), "output: %s", helperOut)
		})
		w.It("a helper only the external test package calls runs", func(it *gotest.T) {
			gotest.Equal(it, 0, helper.Exit, "output: %s", helperOut)
			gotest.Equal(it, "pass", helper.Verdict("TestExtTestSuite/TestCallsExported"), "output: %s", helperOut)
		})
		w.It("a type a non-test file declares is left alone", func(it *gotest.T) {
			gotest.Equal(it, 0, helper.Exit, "output: %s", helperOut)
			gotest.NotContains(it, helperOut, "Contract.TestRoundTrip")
		})
	})
}

func (s *NeverRunTestSuite) TestGenericSuites(t *gotest.T) {
	t.When("a test file declares a generic suite", func(w *gotest.T) {
		w.It("one no alias instantiates is refused at its declaration, told to declare an alias", func(it *gotest.T) {
			checkShape(it, shape{pkg: "orphangeneric", exit: 2,
				shows: `orphangeneric_suite_test\.go:6:6: generic suite OrphanContractTestSuite has no instantiation, so its cases never run; declare one as an alias whose name ends in TestSuite \(type XTestSuite = OrphanContractTestSuite\[\.\.\.\]\)`})
		})
		w.It("one an alias instantiates runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "aliasalias", verdict: "TestIntTestSuite/TestZero"})
		})
	})
}

func (s *NeverRunTestSuite) TestBareNames(t *gotest.T) {
	refused, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/shapes/namebare/")
	gotest.NoError(t, err)
	out := refused.Stdout + refused.Stderr

	t.When("a suite declares a method named exactly Test, Benchmark or Fuzz", func(w *gotest.T) {
		w.It("one taking the T, B or F of that case is refused at the method, told to add a name", func(it *gotest.T) {
			gotest.Equal(it, 2, refused.Exit, "output: %s", out)
			for _, row := range []struct{ at, name string }{
				{"10:25", "Test"}, {"12:25", "Benchmark"}, {"14:25", "Fuzz"},
			} {
				gotest.Contains(it, out, "namebare_suite_test.go:"+row.at+": "+row.name+" on BareTestSuite is no "+row.name+": go test runs no "+row.name+" without a name after the prefix; add one, or give the helper a name that is not "+row.name)
			}
		})
		w.It("one taking no such parameter is a helper and the suite runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "namebarehelper", verdict: "TestHelperNamesTestSuite/TestRuns"})
		})
	})
}

func (s *NeverRunTestSuite) TestConstrainedCallers(t *gotest.T) {
	t.When("the only caller of a helper, or the only alias of a generic suite, is in a test file a build constraint leaves out", func(w *gotest.T) {
		w.It("a run with the build tag off runs the package", func(it *gotest.T) {
			checkShape(it, shape{pkg: "orphantags", verdict: "TestUnitTestSuite/TestA"})
		})
		w.It("a run on a platform the file is not for runs the package", func(it *gotest.T) {
			checkShape(it, shape{pkg: "orphangoos", verdict: "TestUnitTestSuite/TestA"})
		})
		w.It("a run with the build tag on runs the constrained suites too", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/orphantags/", "--tags=orphantagsint")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestIntegrationTestSuite/TestDB"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestPgStoreTestSuite/TestPut"), "stdout: %s", run.Stdout)
		})
		w.It("a helper no file names is still refused", func(it *gotest.T) {
			checkShape(it, shape{pkg: "orphantagsnone", exit: 2,
				shows: `helper_test\.go:8:19: unnamed\.TestNobody takes \*gotest\.T, but unnamed is not a test suite`})
		})
	})
}
