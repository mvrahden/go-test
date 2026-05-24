package about_test

import (
	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GitTestSuite tests PSuiteRegex pattern matching for generated suite file discovery.
type GitTestSuite struct{}

func (s *GitTestSuite) TestPSuiteRegex(t *gotest.T) {
	t.When("generated suite filenames", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct{ Name string }{
			{"ƒƒ_psuite_test.go"},
			{"ƒƒ_pxsuite_test.go"},
			{"gosuite/ƒƒ_psuite_test.go"},
			{"gosuite/ƒƒ_pxsuite_test.go"},
		}) {
			gotest.True(sub, about.PSuiteRegex.Match([]byte(tc.Name)))
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
			gotest.False(sub, about.PSuiteRegex.Match([]byte(tc.Name)))
		}
	})
}
