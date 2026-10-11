package gotestgen_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LocationTestSuite compiles generated harnesses and checks the file:line a
// failing assertion is reported at.
type LocationTestSuite struct{}

func (s *LocationTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *LocationTestSuite) TestInlinedHook(t *gotest.T) {
	t.When("a generic hook is inlined into the generated wrapper", func(w *gotest.T) {
		run := runGeneratedSuite(w, "TestLocation_InlinedGenericHook")

		w.It("reports the assertion in the hook, not in the harness", func(it *gotest.T) {
			// test.go:20 is the hook's gotest.Fail in the fixture source.
			for _, suite := range []string{"TestValueTestSuite", "TestPointerTestSuite"} {
				gotest.Regexp(it, `(?m)^=== RUN   `+suite+`/TestOne\n\s+test\.go:20: Fail$`, run.output, suite)
			}
			gotest.NotContains(it, run.output, "gotest_psuite_test.go:", run.output)
		})
	})
}
