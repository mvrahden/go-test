package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StaleGeneratedTestSuite: a generated file left on disk by an earlier
// generate is read as a bare package clause by every load and never
// compiled into a run.
type StaleGeneratedTestSuite struct{}

func (s *StaleGeneratedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *StaleGeneratedTestSuite) TestStaleFiles(t *gotest.T) {
	t.When("a package holds a generated file from an earlier generate", func(w *gotest.T) {
		w.It("a stale generated file on disk neither breaks the run nor is compiled", func(it *gotest.T) {
			checkShape(it, shape{pkg: "stalerename", verdict: "TestNewTestSuite/TestRuns"})
		})
		w.It("discover and spec read past a stale generated file", func(it *gotest.T) {
			checkReads(it, "stalerename")
		})
	})
}

func (s *StaleGeneratedTestSuite) TestBuildTags(t *gotest.T) {
	t.When("a suite file carries a build constraint", func(w *gotest.T) {
		w.It("--tags= selects the files built with that tag", func(it *gotest.T) {
			for _, flag := range [][]string{{"--tags=foo"}, {"--tags", "foo"}, {"-tags=foo"}} {
				run, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/staletags/", flag...)
				gotest.NoError(it, err)
				gotest.Equal(it, 0, run.Exit, "%v\nstdout: %s\nstderr: %s", flag, run.Stdout, run.Stderr)
				gotest.Equal(it, "pass", run.Verdict("TestTaggedTestSuite/TestRuns"), "%v\nstdout: %s", flag, run.Stdout)
			}
			untagged, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/shapes/staletags/")
			gotest.NoError(it, err)
			gotest.Equal(it, "pass", untagged.Verdict("TestUntaggedTestSuite/TestRuns"), "stdout: %s", untagged.Stdout)
			gotest.Zero(it, untagged.Verdict("TestTaggedTestSuite/TestRuns"), "stdout: %s", untagged.Stdout)
		})
	})
}

// checkReads checks discover and spec --static list the package's current
// suite, NewTestSuite.
func checkReads(t *gotest.T, pkg string) {
	dir := "../testdata/shapes/" + pkg + "/"
	disc, err := testkit.RunCLI(t.Context(), ".", "discover", dir)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, disc.Exit, "stdout: %s\nstderr: %s", disc.Stdout, disc.Stderr)
	gotest.Contains(t, disc.Stdout, `"name":"NewTestSuite"`)
	gotest.NotContains(t, disc.Stdout+disc.Stderr, "OldTestSuite")

	spec, err := testkit.RunCLI(t.Context(), ".", "spec", "--static", dir)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, spec.Exit, "stdout: %s\nstderr: %s", spec.Stdout, spec.Stderr)
	gotest.Regexp(t, `(?m)^New$`, spec.Stdout)
}
