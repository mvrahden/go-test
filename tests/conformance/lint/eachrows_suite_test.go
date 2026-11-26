package lint_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachRowGuardsTestSuite: fail-guard rewrites a guard on an Each row's T only
// where the assertion, which goes on there too, keeps what the guard did.
type EachRowGuardsTestSuite struct{}

func (s *EachRowGuardsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const eachRowsPkg = "../testdata/linteachrows/"

func (s *EachRowGuardsTestSuite) TestFailureGuards(t *gotest.T) {
	t.When("a guard on an Each row's T ends the method", func(w *gotest.T) {
		w.It("fail-guard leaves it unreported, through an alias and inside a closure too", func(it *gotest.T) {
			got := findingsOf(ruleFindings(it, ".", eachRowsPkg), "fail-guard ")
			for _, line := range []string{"19", "22", "27", "31"} {
				gotest.Empty(it, findingsAt(got, "rows_suite_test.go:"+line+":"), "line %s", line)
			}
		})
	})
	t.When("an or-chained or else-if guard fails an Each row's T", func(w *gotest.T) {
		w.It("fail-guard leaves it unreported", func(it *gotest.T) {
			got := findingsOf(ruleFindings(it, ".", eachRowsPkg), "fail-guard ")
			gotest.Empty(it, findingsAt(got, "rows_suite_test.go:40:"))
			gotest.Empty(it, findingsAt(got, "rows_suite_test.go:43:"))
		})
	})
	t.When("a guard of one condition calls Fail or Errorf on an Each row's T", func(w *gotest.T) {
		w.It("fail-guard reports it with a fix and no claim that an assertion halts", func(it *gotest.T) {
			_, fixable := lintFindings(it, ".", eachRowsPkg)
			gotest.Equal(it, []string{
				"fail-guard rows_suite_test.go:53:3: use NoError instead of if+Fail for error nil check",
				"fail-guard rows_suite_test.go:56:3: use NoError instead of if+Errorf for error nil check",
			}, findingsOf(fixable, "fail-guard "))
		})
	})
}
