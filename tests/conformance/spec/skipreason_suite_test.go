package spec_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecSkipReasonTestSuite: a skipped row says why.
type SpecSkipReasonTestSuite struct{}

func (s *SpecSkipReasonTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second, Parallel: true}
}

func (s *SpecSkipReasonTestSuite) TestARowSkippedWithAReason(t *gotest.T) {
	t.When("a run skips a row with a reason", func(w *gotest.T) {
		w.It("the terminal spec shows the reason after SKIPPED", func(it *gotest.T) {
			run := spec(it, 1, "../testdata/eachrows/")
			gotest.Contains(it, run.Stdout, "second — SKIPPED: the second row is not for this run")
		})
		w.It("the Markdown spec shows the reason after the row", func(it *gotest.T) {
			md := spec(it, 1, "--format=md", "../testdata/eachrows/")
			gotest.Contains(it, md.Stdout, "**SKIP** second — the second row is not for this run")
		})
	})
}

func (s *SpecSkipReasonTestSuite) TestAGuardedSuite(t *gotest.T) {
	t.When("a run's suite is kept from running by its guard", func(w *gotest.T) {
		w.It("the terminal spec shows the guard's reason on the suite's line", func(it *gotest.T) {
			guard := spec(it, 0, "../testdata/declaredguard/")
			gotest.Contains(it, guard.Stdout, "Guarded — SKIPPED: suite guard: the guard keeps this suite from running")
		})
	})
}

func (s *SpecSkipReasonTestSuite) TestARowThatLogsBeforeItSkips(t *gotest.T) {
	t.When("a run's row logs a line and then skips", func(w *gotest.T) {
		w.It("the spec shows the skip's reason, not the logged line", func(it *gotest.T) {
			run := spec(it, 0, "../testdata/skiplog/")
			gotest.Regexp(it, `LogsThenSkips — SKIPPED: no database here \(\S+\)\n`, run.Stdout)
			gotest.NotContains(it, run.Stdout, "SKIPPED: connecting to the database")
			md := spec(it, 0, "--format=md", "../testdata/skiplog/")
			gotest.Regexp(it, `\*\*LogsThenSkips\*\* — no database here \(\S+\)\n`, md.Stdout)
		})
		w.It("shows no reason when the skip gave none", func(it *gotest.T) {
			run := spec(it, 0, "../testdata/skiplog/")
			gotest.Regexp(it, `LogsThenSkipsNow — SKIPPED \(\S+\)\n`, run.Stdout)
			gotest.NotContains(it, run.Stdout, "checked the flag")
			md := spec(it, 0, "--format=md", "../testdata/skiplog/")
			gotest.Regexp(it, `\*\*LogsThenSkipsNow\*\* \(\S+\)\n`, md.Stdout)
		})
	})
}
