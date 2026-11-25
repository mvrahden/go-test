package lint_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterSoftTestSuite: lint reads an assertion given t.Soft() as one that
// goes on after failing.
type LinterSoftTestSuite struct{}

func (s *LinterSoftTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const softPkg = "../testdata/lintsoft/"

func (s *LinterSoftTestSuite) TestSoftAssertions(t *gotest.T) {
	t.When("a polling callback asserts on the test's t.Soft(), hands it to a helper and calls Errorf on it", func(w *gotest.T) {
		w.It("lint reports each as a poll-scope finding that names t", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"poll-scope soft_suite_test.go:18:16: use poll instead of t in poll callback passed to Eventually",
				"poll-scope soft_suite_test.go:19:9: t passed to check in poll callback bypasses assertion recording — use poll",
				"poll-scope soft_suite_test.go:20:3: t.Errorf in poll callback bypasses assertion recording — use poll",
			}, findingsOf(ruleFindings(it, ".", softPkg), "poll-scope "))
		})
	})
	t.When("a fatal assertion is followed by a stronger soft one on the same operand", func(w *gotest.T) {
		w.It("assertion-redundant leaves the pair unreported", func(it *gotest.T) {
			gotest.Empty(it, findingsAt(ruleFindings(it, ".", softPkg), "soft_suite_test.go:26:"))
		})
		w.It("assertion-redundant reports the pair when both are soft", func(it *gotest.T) {
			gotest.Equal(it, []string{"assertion-redundant soft_suite_test.go:28:2: Error is redundant before ErrorIs — ErrorIs already checks for non-nil error"},
				findingsOf(ruleFindings(it, ".", softPkg), "assertion-redundant "))
		})
	})
	t.When("a failure guard calls Fail on t.Soft()", func(w *gotest.T) {
		w.It("fail-guard leaves it unreported, an or-chained one too", func(it *gotest.T) {
			// The guards on lines 40 and 43 call FailNow and Errorf.
			gotest.Equal(it, []string{
				"fail-guard soft_suite_test.go:40:2: use NotEqual instead of if+FailNow for == comparison",
				"fail-guard soft_suite_test.go:43:2: use NotEqual instead of if+Errorf for == comparison — assertions halt where Errorf continues",
			}, findingsOf(ruleFindings(it, ".", softPkg), "fail-guard "))
		})
	})
	t.When("lint -fix rewrites assertions and guards on t.Soft()", func(w *gotest.T) {
		root, err := testkit.RepoRoot()
		gotest.NoError(w, err)
		dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "lintsoft"), w.TempDir(), "lintsoftmod")
		gotest.NoError(w, err)
		fix, err := testkit.RunCLI(w.Context(), dir, "lint", "-fix", ".")
		gotest.NoError(w, err)
		src, err := os.ReadFile(filepath.Join(dir, "soft_suite_test.go"))
		gotest.NoError(w, err)

		w.It("True(t.Soft(), a == b) becomes Equal(t.Soft(), a, b)", func(it *gotest.T) {
			gotest.Contains(it, string(src), "\n\tgotest.Equal(t.Soft(), a, b)\n", "stderr: %s", fix.Stderr)
		})
		w.It("a guard calling FailNow on t.Soft() becomes an assertion on t", func(it *gotest.T) {
			gotest.Contains(it, string(src), "\n\tgotest.NotEqual(t, a, b)\n", "stderr: %s", fix.Stderr)
		})
	})
}
