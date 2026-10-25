package gotestgen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// MethodBudgetTestSuite runs generated suites whose Timeout a per-method hook
// or the body overruns, and FailFast suites, and checks the verdicts.
type MethodBudgetTestSuite struct{}

func (s *MethodBudgetTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

const methodBudgetPkg = "TestLifecycle_MethodBudget"

// siteLine is the line of the source that carries the comment SITE:name.
func siteLine(t *gotest.T, name string) string {
	src, err := os.ReadFile(filepath.Join(gotestgen.ExportTestPkgDir(t.T(), methodBudgetPkg), "test.go"))
	gotest.NoError(t, err)
	for i, line := range strings.Split(string(src), "\n") {
		if strings.HasSuffix(line, "// SITE:"+name) {
			return strconv.Itoa(i + 1)
		}
	}
	gotest.Fail(t, "no SITE:%s in the source", name)
	return ""
}

// verdictAt matches the overrun verdict of phase located at the SITE line.
func verdictAt(t *gotest.T, site, phase string) string {
	return `(?m)^\s+test\.go:` + siteLine(t, site) + `: ` + regexp.QuoteMeta(phase) + `exceeded its configured Timeout of 300ms`
}

func (s *MethodBudgetTestSuite) TestAHookOverrunsTheTimeout(t *gotest.T) {
	run := runGeneratedSuiteArgs(t, methodBudgetPkg, childTimeout, "-test.run=^Test(FixtureWaits|BeforeEachWaits|AfterEachSleeps|FixtureAfterEachSleeps|BodySleeps)TestSuite$")
	gotest.False(t, run.passed, run.output)

	t.When("a fixture BeforeEach waits on its context", func(w *gotest.T) {
		w.It("hands it the method's context, which ends at the Timeout, as do the contexts derived from it", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:fixture BeforeEach saw context deadline exceeded, cause context deadline exceeded", run.output)
		})
		w.It("fails the method located at the hook, naming it", func(it *gotest.T) {
			gotest.Regexp(it, verdictAt(it, "fixture-before", "WaitingFixture.BeforeEach "), run.output)
		})
		w.It("runs neither the suite's BeforeEach nor the body, and still runs the fixture's AfterEach with a live context", func(it *gotest.T) {
			out := methodOutput(run.output, "TestFixtureWaitsTestSuite/TestOne")
			gotest.Contains(it, run.output, "--- FAIL: TestFixtureWaitsTestSuite/TestOne", run.output)
			gotest.NotContains(it, out, "MARK:suite BeforeEach", run.output)
			gotest.NotContains(it, out, "MARK:body", run.output)
			gotest.Contains(it, out, "MARK:fixture AfterEach ctx <nil>", run.output)
		})
	})

	t.When("the suite's BeforeEach waits on its context", func(w *gotest.T) {
		w.It("fails the method located at the hook before the hook returns, and never starts the body", func(it *gotest.T) {
			gotest.Contains(it, run.output, "MARK:suite BeforeEach saw context deadline exceeded, cause context deadline exceeded", run.output)
			gotest.Regexp(it, verdictAt(it, "suite-before", "BeforeEach "), run.output)
			gotest.Contains(it, run.output, "--- FAIL: TestBeforeEachWaitsTestSuite/TestOne", run.output)
			gotest.NotContains(it, methodOutput(run.output, "TestBeforeEachWaitsTestSuite/TestOne"), "MARK:body", run.output)
		})
	})

	t.When("an AfterEach overruns the Timeout", func(w *gotest.T) {
		w.It("fails the method located at the suite's or the fixture's hook, naming it", func(it *gotest.T) {
			gotest.Regexp(it, verdictAt(it, "suite-after", "AfterEach "), run.output)
			gotest.Regexp(it, verdictAt(it, "fixture-after", "SleepingFixture.AfterEach "), run.output)
		})
	})

	t.When("the body overruns the Timeout", func(w *gotest.T) {
		w.It("writes the verdict to stderr with its location", func(it *gotest.T) {
			gotest.Regexp(it, `(?m)^gotest: test\.go:`+siteLine(it, "body")+`: TestBodySleepsTestSuite/TestOne exceeded its configured Timeout of 300ms`, run.output)
		})
		w.It("writes a hook's verdict to stderr with its location and its name", func(it *gotest.T) {
			gotest.Regexp(it, `(?m)^gotest: test\.go:`+siteLine(it, "suite-after")+`: TestAfterEachSleepsTestSuite/TestOne AfterEach exceeded its configured Timeout of 300ms`, run.output)
		})
	})
	t.It("never names a runtime file", func(it *gotest.T) {
		gotest.NotContains(it, run.output, "methods.go:", run.output)
		gotest.NotContains(it, run.output, "harness.go:", run.output)
	})
}

func (s *MethodBudgetTestSuite) TestFailFast(t *gotest.T) {
	t.When("a sequential FailFast suite skips a method", func(w *gotest.T) {
		run := runGeneratedSuiteArgs(w, methodBudgetPkg, childTimeout, "-test.run=^TestFailFastTestSuite$")
		w.It("names the method that failed and no runtime file", func(it *gotest.T) {
			gotest.Contains(it, run.output, "--- SKIP: TestFailFastTestSuite/TestB", run.output)
			gotest.Regexp(it, `(?m)^\s+FailFast: TestA$`, run.output)
			gotest.NotContains(it, run.output, "methods.go:", run.output)
		})
	})

	t.When("a parallel method fails only in its cleanup", func(w *gotest.T) {
		run := runGeneratedSuiteArgs(w, methodBudgetPkg, childTimeout, "-test.run=^TestParallelFailFastTestSuite$", "-test.parallel=1")
		w.It("stops the method that had not started", func(it *gotest.T) {
			gotest.False(it, run.passed, run.output)
			gotest.Equal(it, 1, strings.Count(run.output, "MARK:ran "), run.output)
			gotest.Equal(it, 1, strings.Count(run.output, "--- SKIP: TestParallelFailFastTestSuite/"), run.output)
			gotest.Regexp(it, `(?m)^\s+FailFast: Test[AB]$`, run.output)
			gotest.NotContains(it, run.output, "methods.go:", run.output)
		})
	})
}

// methodOutput is what -test.v printed between test's RUN line and its
// verdict.
func methodOutput(output, test string) string {
	_, rest, _ := strings.Cut(output, "=== RUN   "+test+"\n")
	part, _, _ := strings.Cut(rest, " "+test+" (")
	return part
}
