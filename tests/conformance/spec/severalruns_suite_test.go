package spec_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecSeveralRunsTestSuite: a test or package that failed in one of the
// runs a stream holds reads failed, whichever run came last.
type SpecSeveralRunsTestSuite struct{}

func (s *SpecSeveralRunsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

const (
	redThenGreen     = "--input=../testdata/specstreams/failedthenpassed.jsonl"
	greenThenRed     = "--input=../testdata/specstreams/passedthenfailed.jsonl"
	crashedThenGreen = "--input=../testdata/specstreams/crashedthenpassed.jsonl"
)

// withoutRunEvents drops the events a run booked for itself, leaving the
// stream an older gotest wrote.
func withoutRunEvents(stream string) string {
	var kept strings.Builder
	for line := range strings.Lines(stream) {
		if !strings.Contains(line, `"RunLevel":true`) {
			kept.WriteString(line)
		}
	}
	return kept.String()
}

func (s *SpecSeveralRunsTestSuite) TestAFailedRunFollowedByAPassedOne(t *gotest.T) {
	t.When("a stream holds a package's failed run and, after it, a run in which the same tests passed", func(w *gotest.T) {
		w.It("renders the failed method, its suite and its package as failed, with the failure's message, in the terminal, Markdown and JSON formats", func(it *gotest.T) {
			terminal := spec(it, 1, redThenGreen)
			gotest.Regexp(it, `(?m)^  ✗ Posts `, terminal.Stdout)
			gotest.Contains(it, terminal.Stdout, "ledger_test.go:14: posted twice")
			md := spec(it, 1, "--format=md", redThenGreen)
			gotest.Contains(it, md.Stdout, "- **FAIL** example.com/again › Ledger › Posts")
			doc := specJSON(it, 1, redThenGreen)
			gotest.Len(it, doc.Packages, 1)
			gotest.Equal(it, "fail", doc.Packages[0].Status)
			gotest.Equal(it, "fail", node(it, doc.Packages[0].Nodes, "TestLedgerTestSuite").Status)
			gotest.Equal(it, "fail", node(it, doc.Packages[0].Nodes, "TestLedgerTestSuite", "TestPosts").Status)
			gotest.Equal(it, "pass", node(it, doc.Packages[0].Nodes, "TestLedgerTestSuite", "TestBalances").Status)
		})
		w.It("counts one failure and exits 1, or 0 with --render-only", func(it *gotest.T) {
			doc := specJSON(it, 1, redThenGreen)
			gotest.Equal(it, 1, doc.Stats.Failed)
			gotest.Equal(it, 1, doc.Stats.Passed)
			gotest.Regexp(it, `1 passed, 1 failed\n$`, spec(it, 1, redThenGreen).Stdout)
			spec(it, 0, "--render-only", redThenGreen)
			// Two real runs appended: as written, and as an older gotest wrote them.
			failed, err := testkit.RunPackageWithEnv(it.Context(), it.TempDir(), []string{"SUMMARYMIX=fail"}, "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Equal(it, 1, failed.Exit, "stderr: %s", failed.Stderr)
			passed, err := testkit.RunPackage(it.Context(), it.TempDir(), "../testdata/summarymix/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, passed.Exit, "stderr: %s", passed.Stderr)
			appended := failed.Stdout + passed.Stdout
			for _, stream := range []string{appended, withoutRunEvents(appended)} {
				file := filepath.Join(it.TempDir(), "runs.json")
				gotest.NoError(it, os.WriteFile(file, []byte(stream), 0o600))
				real := specJSON(it, 1, "--input="+file)
				gotest.Equal(it, 1, real.Stats.Failed)
				gotest.Equal(it, 1, real.Stats.Passed)
				gotest.Equal(it, 2, real.Stats.Skipped)
				summary, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--input="+file)
				gotest.NoError(it, err)
				gotest.Equal(it, 1, summary.Exit, "stderr: %s", summary.Stderr)
				gotest.Regexp(it, `^1 test failed, 1 passed, 2 skipped\n`, summary.Stdout)
			}
		})
	})
}

func (s *SpecSeveralRunsTestSuite) TestAPassedRunFollowedByAFailedOne(t *gotest.T) {
	t.When("a stream holds a package's passed run and, after it, a run in which one of the same tests failed", func(w *gotest.T) {
		w.It("renders the failed method as failed and counts one failure", func(it *gotest.T) {
			doc := specJSON(it, 1, greenThenRed)
			gotest.Equal(it, "fail", node(it, doc.Packages[0].Nodes, "TestLedgerTestSuite", "TestPosts").Status)
			gotest.Equal(it, 1, doc.Stats.Failed)
			gotest.Equal(it, 1, doc.Stats.Passed)
			gotest.Contains(it, spec(it, 1, greenThenRed).Stdout, "ledger_test.go:14: posted twice")
		})
	})
}

func (s *SpecSeveralRunsTestSuite) TestAPackageThatFailedOnItsOwnThenPassed(t *gotest.T) {
	t.When("a stream holds a run in which a package failed with every test passing and, after it, a run in which the package passed", func(w *gotest.T) {
		w.It("renders the package as failed, with its output, counts one failed package and exits 1", func(it *gotest.T) {
			doc := specJSON(it, 1, crashedThenGreen)
			gotest.Equal(it, "fail", doc.Packages[0].Status)
			gotest.Equal(it, 1, doc.Stats.FailedPackages)
			gotest.Equal(it, 0, doc.Stats.Failed)
			gotest.Equal(it, 2, doc.Stats.Passed)
			terminal := spec(it, 1, crashedThenGreen)
			gotest.Contains(it, terminal.Stdout, "panic: the ledger is closed")
			gotest.Regexp(it, `2 passed, 1 failed package\n$`, terminal.Stdout)
		})
	})
}
