package about_test

import (
	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GitTestSuite tests PSuiteRegex pattern matching for generated suite file discovery.
type GitTestSuite struct{}

func (s *GitTestSuite) TestPSuiteRegex(t *gotest.T) {
	t.When("generated suite filenames", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{"gotest_psuite_test.go"},
			{"gotest_pxsuite_test.go"},
			{"gosuite/gotest_psuite_test.go"},
			{"gosuite/gotest_pxsuite_test.go"},
		}) {
			gotest.Regexp(sub, about.PSuiteRegex, tc.Name)
		}
	})

	t.When("non-suite filenames", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{"ptest_test.go"},
			{"pxtest_test.go"},
			{"focus_suite/main.go"},
			{"simple_suite/ptest_test.go"},
			{"simple_suite/pxtest_test.go"},
			{"focus_suite/gotestgen_ptest.golden"},
			{"focus_suite/gotestgen_pxtest.golden"},
		}) {
			gotest.False(sub, about.PSuiteRegex.MatchString(tc.Name))
		}
	})
}
