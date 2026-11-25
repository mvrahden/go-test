package assertions_test

import (
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SoftAssertionsTestSuite: an assertion given t.Soft() reports where a fatal
// one would and lets the test go on.
type SoftAssertionsTestSuite struct{}

func (s *SoftAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

const softPkg = "../testdata/softasserts/"

func (s *SoftAssertionsTestSuite) TestSoftAssertions(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), softPkg)
	gotest.NoError(t, err)
	t.When("soft assertions fail in a method", func(w *gotest.T) {
		w.It("each failure names its assertion and the line it failed at", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `softasserts_suite_test\.go:26: Equal failed`, run.Stdout)
			gotest.Regexp(it, `softasserts_suite_test\.go:27: True failed`, run.Stdout)
		})
		w.It("the method goes on past them and fails", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "after the soft failures")
			gotest.Equal(it, "fail", run.Verdict("TestSoftTestSuite/TestMethod"))
		})
	})
	t.When("an It runs on a soft T", func(w *gotest.T) {
		w.It("an assertion that fails in its body ends the It there", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSoftTestSuite/TestIt/stops"))
			gotest.NotContains(it, run.Markers, "after the failure in the It")
			gotest.Contains(it, run.Markers, "after the It")
		})
	})
	t.When("a soft assertion fails in BeforeEach", func(w *gotest.T) {
		w.It("BeforeEach goes on, the method runs and fails", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("BeforeEach went on", "the method ran"), "markers: %v", run.Markers)
			gotest.Equal(it, "fail", run.Verdict("TestSoftHookTestSuite/TestRuns"))
			gotest.Regexp(it, `softasserts_suite_test\.go:58: True failed`, run.Stdout)
			gotest.Contains(it, run.Stdout, "message: BeforeEach fails")
		})
	})
	t.When("Eventually is given a soft T and no poll passes", func(w *gotest.T) {
		w.It("the test goes on past it and fails", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "after Eventually")
			gotest.Equal(it, "fail", run.Verdict("TestSoftTestSuite/TestEventually"))
			gotest.Regexp(it, `Eventually failed after 50ms`, run.Stdout)
		})
	})
}

func (s *SoftAssertionsTestSuite) TestSoftHelperUnderTrimpath(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "softasserts"), staged, "plainmod")
	gotest.NoError(t, err)
	run, err := testkit.RunCLI(t.Context(), staged, "-trimpath", "-v", "-run", "TestSoftTestSuite/TestHelper", "./softasserts/")
	gotest.NoError(t, err)
	t.When("a soft assertion fails in a helper of a module whose path has no dot, built with -trimpath", func(w *gotest.T) {
		w.It("the soft failure names the helper's line and the caller's, and the test goes on", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `(?m)^\s+softasserts_suite_test\.go:37: softasserts_suite_test\.go:32: Equal failed`, run.Stdout)
			gotest.Regexp(it, `(?m)^\s+softasserts_suite_test\.go:33: Equal failed`, run.Stdout)
			gotest.NotContains(it, run.Stderr, "Equal failed")
		})
	})
}
