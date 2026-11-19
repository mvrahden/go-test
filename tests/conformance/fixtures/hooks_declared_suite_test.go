package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureHookDeclarationsTestSuite: a package fixture's BeforeAll or AfterAll
// that fails is reported where the hook is declared.
type FixtureHookDeclarationsTestSuite struct{}

func (s *FixtureHookDeclarationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *FixtureHookDeclarationsTestSuite) TestWhatAFailedFixtureBeforeAllOrAfterAllSays(t *gotest.T) {
	t.When("a package fixture's BeforeAll returns an error", func(w *gotest.T) {
		run, _ := runHooks(w, "fixture.BeforeAll", "error")
		w.It("is reported at the hook's declaration, not in the runtime's file", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, failedAt(it, "fixture.go", "hooks/fixture.go", "func (f *GridFixture) BeforeAll", "fixture setup: GridFixture.BeforeAll: fixture.BeforeAll failed"), outputOf(&run, gridSuite))
		})
	})
	t.When("a package fixture's AfterAll returns an error", func(w *gotest.T) {
		run, _ := runHooks(w, "fixture.AfterAll", "error")
		w.It("is reported at the hook's declaration, not in the runtime's file", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, failedAt(it, "fixture.go", "hooks/fixture.go", "func (f *GridFixture) AfterAll", "fixture teardown failed: GridFixture.AfterAll: fixture.AfterAll failed"), outputOf(&run, gridSuite))
		})
	})
}
