package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryFocusTestSuite: focus, by a suite or a method, is decided over the
// whole run, not per package or test package variant.
type DiscoveryFocusTestSuite struct{}

func (s *DiscoveryFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *DiscoveryFocusTestSuite) TestRunWide(t *gotest.T) {
	t.When("a run holds a focused suite", func(w *gotest.T) {
		w.It("a focused suite in one package skips every unfocused suite in the others", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/focusacross/...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestF_OnlyTestSuite/TestRuns"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Markers, "focused ran")
			gotest.Equal(it, "skip", run.Verdict("TestOtherTestSuite"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestOtherTestSuite/TestRuns"))
		})
		w.It("an unfocused suite skipped by focus starts no fixture", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/focusacross/...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "focused ran")
			gotest.NotContains(it, run.Markers, "fixture up")
			gotest.NotContains(it, run.Markers, "shared up")
		})
		w.It("a focused suite in the internal test package skips the external one's suites", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/focusvariants/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestF_InTestSuite/TestRuns"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "skip", run.Verdict("TestExtTestSuite"), "stdout: %s", run.Stdout)
		})
		w.It("--ci still refuses a run with a focused suite", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "--ci", "-json", "../testdata/shapes/focusacross/...")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `FAIL: focus prefix detected — remove F_ before merging:\n\s*\S*a_suite_test\.go:\d+\s+type F_OnlyTestSuite`, run.Stderr)
			gotest.Empty(it, run.Verdict("TestF_OnlyTestSuite/TestRuns"))
		})
	})
	t.When("a run holds a focused method", func(w *gotest.T) {
		w.It("a focused method in one package skips every unfocused suite in the others", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/focusmethod/...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestMethodTestSuite/F_TestFocused"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestMethodTestSuite/TestPlain"))
			gotest.Equal(it, "skip", run.Verdict("TestOtherTestSuite"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestOtherTestSuite/TestRuns"))
		})
		w.It("a focused fuzz method in one package skips every unfocused suite in the others", func(it *gotest.T) {
			run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/focusfuzz/...")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("FuzzSeedsTestSuite_F_FuzzSeeds"), "stdout: %s", run.Stdout)
			gotest.Empty(it, run.Verdict("TestSeedsTestSuite/TestPlain"))
			gotest.Empty(it, run.Verdict("TestOtherTestSuite/TestFails"))
		})
	})
}
