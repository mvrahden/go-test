package audit_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditGuardedRunTestSuite: a test the run skips for a guard the base has
// already is not proven, and no weakening.
type AuditGuardedRunTestSuite struct {
	dir, cache       string
	ran, all, notRun testkit.Run
}

func (s *AuditGuardedRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditGuardedRunTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-guard-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	co := gatesCheckout(t, dir, "guard", "guard")
	s.ran = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--format=json")
	s.all = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--fail-on=all")
	s.notRun = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--fail-on=not-run")
}

func (s *AuditGuardedRunTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditGuardedRunTestSuite) TestAGuardThatHolds(t *gotest.T) {
	t.When("audit --run runs a changed test whose suite's guard, as the base has it, keeps it from running", func(t *gotest.T) {
		t.It("reads the test as skip with the guard's reason, counts it as not run and lists no weakening", func(it *gotest.T) {
			gotest.Equal(it, 0, s.ran.Exit, "stderr: %s", s.ran.Stderr)
			doc := readRunDoc(it, s.ran.Stdout)
			gotest.Len(it, doc.Evidence, 1)
			e := doc.Evidence[0]
			gotest.Equal(it, []string{"Guarded", "Twice"}, e.Path)
			gotest.Equal(it, "skip", e.Verdict)
			gotest.Equal(it, "suite guard: GATESMOD_DB is not set", e.SkipReason)
			gotest.True(it, e.NeedsDecision)
			gotest.Equal(it, 1, doc.Summary.NotRun)
			gotest.Empty(it, doc.findings(true))
			gotest.Equal(it, 0, doc.Summary.Weakenings)
		})
		t.It("--fail-on=all exits 0 and --fail-on=not-run exits 1", func(it *gotest.T) {
			gotest.Equal(it, 0, s.all.Exit, "stderr: %s", s.all.Stderr)
			gotest.Regexp(it, "(?m)^gotest audit: PASS exit 0 \\| weakenings: 0 \\|", s.all.Stdout)
			gotest.Equal(it, 1, s.notRun.Exit, "stderr: %s", s.notRun.Stderr)
			gotest.Regexp(it, "\\| evidence: 1 \\(failing 0, not run 1, vacuous 0, stale 0\\) \\| fail-on: not-run\\n$", s.notRun.Stdout)
		})
	})
}
