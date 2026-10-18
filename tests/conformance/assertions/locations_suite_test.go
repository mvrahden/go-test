package assertions_test

import (
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailureLocationsTestSuite: a failure names the lines it happened at, with
// whatever the build did to the paths.
type FailureLocationsTestSuite struct {
	cli    string
	staged string
}

func (s *FailureLocationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

// BeforeAll stages the helpers package into a module whose path has no dot,
// the shape -trimpath cannot tell from the standard library by path alone.
func (s *FailureLocationsTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	s.staged = t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "helpers"), s.staged, "plainmod")
	gotest.NoError(t, err)
}

// AfterAll has nothing to release: the binary and the module live in temp dirs.
func (s *FailureLocationsTestSuite) AfterAll(t *gotest.T) {}

func (s *FailureLocationsTestSuite) TestHelperUnderTrimpath(t *gotest.T) {
	run := exec.Command(s.cli, "-trimpath", "-v", "./helpers/") //nolint:gosec // G204: controlled binary with fixed args
	run.Dir = s.staged
	out, _ := run.CombinedOutput()
	t.When("a helper fails in a module whose path has no dot, built with -trimpath", func(w *gotest.T) {
		w.It("the failure names the helper's line and the caller's", func(it *gotest.T) {
			gotest.Regexp(it, `helpers_suite_test\.go:14: helpers_suite_test\.go:10: Equal failed`, string(out))
		})
	})
}
