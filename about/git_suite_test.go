package about_test

import (
	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GitTestSuite tests PSuiteRegex pattern matching for generated suite file discovery.
type GitTestSuite struct{}

func (s *GitTestSuite) TestPSuiteRegex(t *gotest.T) {
	t.When("generated suite filenames", func(w *gotest.T) {
		for _, v := range []string{
			"ƒƒ_psuite_test.go",
			"ƒƒ_pxsuite_test.go",
			"gosuite/ƒƒ_psuite_test.go",
			"gosuite/ƒƒ_pxsuite_test.go",
		} {
			w.It("matches "+v, func(it *gotest.T) {
				gotest.True(it, about.PSuiteRegex.Match([]byte(v)))
			})
		}
	})

	t.When("non-suite filenames", func(w *gotest.T) {
		for _, v := range []string{
			"ptest_test.go",
			"pxtest_test.go",
			"focus_suite/main.go",
			"simple_suite/ptest_test.go",
			"simple_suite/pxtest_test.go",
			"focus_suite/gotestgen_ptest.golden",
			"focus_suite/gotestgen_pxtest.golden",
		} {
			w.It("rejects "+v, func(it *gotest.T) {
				gotest.False(it, about.PSuiteRegex.Match([]byte(v)))
			})
		}
	})
}
