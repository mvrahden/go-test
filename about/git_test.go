package about_test

import (
	"testing"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func Test_Regex(t *testing.T) {
	t.Run("matches generated suite filenames", func(t *testing.T) {
		for _, v := range []string{
			"ƒƒ_psuite_test.go",
			"ƒƒ_pxsuite_test.go",
			"gosuite/ƒƒ_psuite_test.go",
			"gosuite/ƒƒ_pxsuite_test.go",
		} {
			gotest.True(t, about.PSuiteRegex.Match([]byte(v)), "failed for %q", v)
		}
	})
	t.Run("rejects non-suite filenames", func(t *testing.T) {
		for _, v := range []string{
			"ptest_test.go",
			"pxtest_test.go",
			"focus_suite/main.go",
			"simple_suite/ptest_test.go",
			"simple_suite/pxtest_test.go",
			"focus_suite/gotestgen_ptest.golden",
			"focus_suite/gotestgen_pxtest.golden",
		} {
			gotest.False(t, about.PSuiteRegex.Match([]byte(v)), "failed for %q", v)
		}
	})
}
