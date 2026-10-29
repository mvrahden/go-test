package assertions_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailureLocationsTestSuite: a failure names the lines it happened at, with
// whatever the build did to the paths.
type FailureLocationsTestSuite struct{}

func (s *FailureLocationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute}
}

func (s *FailureLocationsTestSuite) TestHelperUnderTrimpath(t *gotest.T) {
	// A module whose path has no dot: the shape -trimpath cannot tell from
	// the standard library by path alone.
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "helpers"), staged, "plainmod")
	gotest.NoError(t, err)
	run, err := testkit.RunCLI(t.Context(), staged, "-trimpath", "-v", "./helpers/")
	gotest.NoError(t, err)
	t.When("a helper fails in a module whose path has no dot, built with -trimpath", func(w *gotest.T) {
		w.It("the failure names the helper's line and the caller's", func(it *gotest.T) {
			gotest.Regexp(it, `helpers_suite_test\.go:14: helpers_suite_test\.go:10: Equal failed`, run.Stdout+run.Stderr)
		})
	})
}

func (s *FailureLocationsTestSuite) TestHelperInADottedElementModuleUnderTrimpath(t *gotest.T) {
	// A module path whose first element has no dot but whose last has one,
	// with the helper in the module's root package: its frames read
	// corp/tools%2ev2.expectAnswer.
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "helpers", "helpers_suite_test.go"))
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(filepath.Join(staged, "helpers_suite_test.go"), data, 0o600))
	gotest.NoError(t, testkit.WriteModule(root, staged, "corp/tools.v2"))
	run, err := testkit.RunCLI(t.Context(), staged, "-trimpath", "-v", "./")
	gotest.NoError(t, err)
	t.When("a helper fails in a module whose path's first element has no dot and a later one has, built with -trimpath", func(w *gotest.T) {
		w.It("the failure names the helper's line and the caller's", func(it *gotest.T) {
			gotest.Regexp(it, `helpers_suite_test\.go:14: helpers_suite_test\.go:10: Equal failed`, run.Stdout+run.Stderr)
		})
	})
}

func (s *FailureLocationsTestSuite) TestHelperInARequiredModuleUnderTrimpath(t *gotest.T) {
	// Two modules without a dot, one requiring the other through a replace:
	// the build records the second as a dependency, not as the main module,
	// and every user frame of the failing behavior is the dependency's.
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "modhelpers"), staged, "plainmod")
	gotest.NoError(t, err)
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "check"), filepath.Join(staged, "plaincheck"), "plaincheck")
	gotest.NoError(t, err)
	goMod := filepath.Join(staged, "go.mod")
	data, err := os.ReadFile(goMod)
	gotest.NoError(t, err)
	data = append(data, "\nrequire plaincheck v0.0.0\n\nreplace plaincheck => ./plaincheck\n"...)
	gotest.NoError(t, os.WriteFile(goMod, data, 0o600))
	run, err := testkit.RunCLI(t.Context(), staged, "-trimpath", "-v", "./modhelpers/")
	gotest.NoError(t, err)
	t.When("a behavior another module declares fails in its helper, the module's path having no dot, required through a replace and built with -trimpath", func(w *gotest.T) {
		w.It("the failure names the helper's line and the caller's", func(it *gotest.T) {
			gotest.Regexp(it, `check\.go:14: check\.go:9: Equal failed`, run.Stdout+run.Stderr)
		})
	})
}
