package gotestgen_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StaleBrokenLoadTestSuite: a package that does not compile is built once
// more for the compiler's report, and a generated file left on disk is not
// part of that build.
type StaleBrokenLoadTestSuite struct{}

func (s *StaleBrokenLoadTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *StaleBrokenLoadTestSuite) TestTheReportLeavesOutAStaleGeneratedFile(t *gotest.T) {
	_, broken, err := gotestgen.LoadPackages([]string{"../../tests/conformance/testdata/shapes/stalebroken"}, nil)
	gotest.NoError(t, err)
	gotest.Len(t, broken, 1)
	report := strings.Join(broken[0].Errors, "\n")
	t.It("reports the suite file's error", func(it *gotest.T) {
		gotest.Contains(it, report, `cannot use "not an int"`)
	})
	t.It("reports nothing of the stale generated file", func(it *gotest.T) {
		gotest.NotContains(it, report, "OldTestSuite")
	})
}
