package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterFixesTestSuite: lint -fix applies every fix whose result compiles and
// names each one it drops; a target that does not compile is reported once.
type LinterFixesTestSuite struct{}

func (s *LinterFixesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *LinterFixesTestSuite) TestFixThatDoesNotCompile(t *gotest.T) {
	t.When("a fix does not compile", func(w *gotest.T) {
		root, err := testkit.RepoRoot()
		gotest.NoError(w, err)
		dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "lintfixbad"), w.TempDir(), "lintfixmod")
		gotest.NoError(w, err)
		fix, err := testkit.RunCLI(w.Context(), dir, "lint", "-fix", ".")
		gotest.NoError(w, err)
		src, err := os.ReadFile(filepath.Join(dir, "fix_test.go"))
		gotest.NoError(w, err)

		w.It("a fix that does not compile is dropped and named", func(it *gotest.T) {
			gotest.Equal(it, 3, fix.Exit, "stderr: %s", fix.Stderr)
			gotest.Contains(it, fix.Stderr, "fix for receiver at lintfixbad/fix_test.go:11 was not applied: it does not compile (cannot call pointer method TestValue on ValueTestSuite)")
			gotest.Contains(it, string(src), "\nfunc (s ValueTestSuite) TestValue(t *gotest.T) {}\n")
		})
		w.It("the other fixes are applied", func(it *gotest.T) {
			gotest.Contains(it, string(src), "\tgotest.NoError(t, err)\n}\n", "stderr: %s", fix.Stderr)
			gotest.NotContains(it, string(src), "gotest.True")
		})
	})
}

func (s *LinterFixesTestSuite) TestTargetThatDoesNotCompile(t *gotest.T) {
	t.When("a lint target does not compile", func(w *gotest.T) {
		run, err := testkit.RunCLI(w.Context(), ".", "lint", "../testdata/lintfixbroken/")
		gotest.NoError(w, err)

		w.It("a package that does not compile is reported once, with its path relative to the module", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			const msg = `cannot use "none" (untyped string constant) as int value in variable declaration`
			gotest.Equal(it, 1, strings.Count(run.Stderr, msg), "stderr: %s", run.Stderr)
			gotest.Contains(it, run.Stderr, "\ttests/conformance/testdata/lintfixbroken/lintfixbroken.go:4:17: "+msg+"\n")
			const missing = "undefined: lintfixbroken.Missing"
			gotest.Equal(it, 1, strings.Count(run.Stderr, missing), "stderr: %s", run.Stderr)
			gotest.Contains(it, run.Stderr, "\ttests/conformance/testdata/lintfixbroken/count_ext_test.go:9:23: "+missing+"\n")
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			gotest.NotContains(it, run.Stderr, root+string(filepath.Separator))
			gotest.NotContains(it, run.Stderr, "../testdata")
		})
	})
}
