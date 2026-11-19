package lint_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterDroppedGatedTestSuite: a case reached only from a file the build
// constraints leave out, or through an interface, is not a dropped case.
type LinterDroppedGatedTestSuite struct{}

func (s *LinterDroppedGatedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const droppedGatedPkg = "../testdata/lintdroppedgated/"

func (s *LinterDroppedGatedTestSuite) TestCasesReachedWhereTheBuildLeavesThemOut(t *gotest.T) {
	t.When("a helper's only caller or a generic suite's only alias sits in a test file the build constraints leave out, or a helper is handed to an interface that holds its case", func(w *gotest.T) {
		w.It("lint leaves each unreported, as the generator runs it", func(it *gotest.T) {
			got := findingsOf(ruleFindings(it, ".", droppedGatedPkg), "dropped-case ")
			for _, line := range []string{"8", "11", "18"} {
				gotest.Empty(it, findingsAt(got, "a_test.go:"+line+":"), "line %s", line)
			}
		})
		w.It("lint reports beside them a case reached from nowhere", func(it *gotest.T) {
			got := findingsOf(ruleFindings(it, ".", droppedGatedPkg), "dropped-case ")
			gotest.Equal(it, []string{"dropped-case a_test.go:27:1: TestAlone on Lonely never runs: Lonely is no suite; end its name in TestSuite, or give the helper a name that does not begin with Test"}, got)
		})
	})
}
