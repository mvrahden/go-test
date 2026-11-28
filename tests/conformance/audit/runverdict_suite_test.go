package audit_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditRunVerdictTestSuite: the run audit --run makes is judged by its exit
// code, not by its rows alone.
type AuditRunVerdictTestSuite struct {
	dir, cache                   string
	added, addedFailing, addedMd testkit.Run
	// hookOnly changes a hook alone; unchanged changes nothing.
	hookOnly, unchanged testkit.Run
}

func (s *AuditRunVerdictTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditRunVerdictTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-runverdict-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	co := gatesCheckout(t, dir, "hooktest", "hooktest")
	s.added = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--format=json")
	s.addedFailing = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--fail-on=failing")
	s.addedMd = auditIn(t, co, s.cache, "--base=HEAD", "--run", "--format=md")
	s.hookOnly = auditIn(t, gatesCheckout(t, dir, "hook", "hook"), s.cache, "--base=HEAD", "--run", "--fail-on=failing", "--format=json")
	s.unchanged = auditIn(t, gatesCheckout(t, dir, "plain"), s.cache, "--base=HEAD", "--run", "--format=json")
}

func (s *AuditRunVerdictTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditRunVerdictTestSuite) TestARunThatFails(t *gotest.T) {
	t.When("audit --run runs an added test whose suite's BeforeAll fails", func(t *gotest.T) {
		t.It("lists a failing finding about the run naming exit 1, beside the test read as not run, and exits 0 without --fail-on", func(it *gotest.T) {
			gotest.Equal(it, 0, s.added.Exit, "stderr: %s", s.added.Stderr)
			doc := readRunDoc(it, s.added.Stdout)
			gotest.Equal(it, []finding{{"failing", "this audit's run", "", "exited 1"}}, doc.findings(false))
			gotest.Len(it, doc.Findings, 1)
			gotest.False(it, doc.Findings[0].Weakens)
			gotest.Equal(it, 0, doc.Summary.Weakenings)
			gotest.Equal(it, map[string]int{"failing": 1, "not-run": 1}, doc.Summary.Classes)
			gotest.Len(it, doc.Evidence, 1)
			gotest.Equal(it, []string{"Calc", "SubAgain"}, doc.Evidence[0].Path)
			gotest.Equal(it, "not run", doc.Evidence[0].Verdict)
		})
		t.It("--fail-on=failing exits 1 and the closing line names run: exited 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.addedFailing.Exit, "stderr: %s", s.addedFailing.Stderr)
			gotest.Regexp(it, "\\| evidence: 1 \\(failing 0, not run 1, vacuous 0, stale 0\\) \\| run: exited 1 \\| fail-on: failing\\n$", s.addedFailing.Stdout)
			gotest.Regexp(it, "(?m)^gotest audit: FAIL exit 1 \\|", s.addedFailing.Stdout)
		})
		t.It("names the run under run: in the text format and under a Run heading in Markdown", func(it *gotest.T) {
			gotest.Contains(it, s.addedFailing.Stdout, "\n\nrun:\n  failing  this audit's run exited 1\n")
			gotest.Contains(it, s.addedMd.Stdout, "\n#### Run\n\n- failing: this audit's run exited 1\n")
			gotest.Equal(it, 0, s.addedMd.Exit, "stderr: %s", s.addedMd.Stderr)
		})
	})
}

func (s *AuditRunVerdictTestSuite) TestAChangedTestFile(t *gotest.T) {
	t.When("audit --run compares a head that changes only a suite's hook, which now fails", func(t *gotest.T) {
		t.It("runs the suite's package though no behavior changed, and lists the failing run", func(it *gotest.T) {
			doc := readRunDoc(it, s.hookOnly.Stdout)
			gotest.Empty(it, doc.Behaviors)
			gotest.Len(it, doc.Streams, 1)
			gotest.True(it, doc.Streams[0].Ran)
			gotest.NotZero(it, doc.Streams[0].Run)
			gotest.Equal(it, []string{"-json", "gatesmod/calc"}, doc.Streams[0].Run.Args)
			gotest.Equal(it, []finding{{"failing", "this audit's run", "", "exited 1"}}, doc.findings(false))
			gotest.Equal(it, 1, s.hookOnly.Exit, "stderr: %s", s.hookOnly.Stderr)
		})
	})
	t.When("audit --run compares a head that changes no Go file", func(t *gotest.T) {
		t.It("runs nothing and notes that no package holds a changed behavior, a changed test file or a changed production file", func(it *gotest.T) {
			gotest.Equal(it, 0, s.unchanged.Exit, "stderr: %s", s.unchanged.Stderr)
			doc := readRunDoc(it, s.unchanged.Stdout)
			gotest.Empty(it, doc.Streams)
			gotest.Equal(it, []string{"--run: no package holds a changed behavior, a changed test file or a changed production file, so nothing ran"}, doc.Notes)
		})
	})
}
