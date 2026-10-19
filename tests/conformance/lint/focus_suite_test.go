package lint_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterFocusFixTestSuite: lint -fix removes a focus prefix at every
// reference to the focused suite in its package.
type LinterFocusFixTestSuite struct{}

func (s *LinterFocusFixTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// unfocusedFix is a staged copy of a testdata package after lint -fix ran
// on it.
type unfocusedFix struct {
	mod, dir string
	run      testkit.Run
}

func fixFocus(t *gotest.T, pkg string) *unfocusedFix {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	mod := t.TempDir()
	dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", pkg), mod, "lintfocusmod")
	gotest.NoError(t, err)
	run, err := testkit.RunCLI(t.Context(), dir, "lint", "-fix", ".")
	gotest.NoError(t, err)
	return &unfocusedFix{mod: mod, dir: dir, run: run}
}

func (c *unfocusedFix) read(t *gotest.T, file string) string {
	src, err := os.ReadFile(filepath.Join(c.dir, file))
	gotest.NoError(t, err)
	return string(src)
}

// vet type-checks the staged module, test files included.
func (c *unfocusedFix) vet(t *gotest.T) {
	cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = c.mod
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go vet: %s", out)
}

var focusPrefix = regexp.MustCompile(`F_\w+`)

// word matches name as a whole identifier.
func word(name string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + name + `\b`)
}

func (s *LinterFocusFixTestSuite) TestSpread(t *gotest.T) {
	t.When("lint -fix removes the focus of a suite whose methods and uses are spread over several files", func(w *gotest.T) {
		c := fixFocus(w, "lintfocusspread")
		w.It("lint -fix renames every receiver and use, and the package still compiles", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			var all string
			for _, f := range []string{"suite_test.go", "more_test.go", "helpers_test.go"} {
				src := c.read(it, f)
				gotest.Empty(it, focusPrefix.FindAllString(src, -1), "%s:\n%s", f, src)
				all += src
			}
			// The declaration, four receivers and a literal.
			gotest.Len(it, word("SpreadTestSuite").FindAllString(all, -1), 6, all)
			c.vet(it)
		})
	})
}

func (s *LinterFocusFixTestSuite) TestGeneric(t *gotest.T) {
	t.When("lint -fix removes the focus of a generic suite that aliases instantiate", func(w *gotest.T) {
		c := fixFocus(w, "lintfocusgeneric")
		w.It("lint -fix renames every generic receiver and alias, and the package still compiles", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			var all string
			for _, f := range []string{"contract_test.go", "more_test.go", "aliases_test.go"} {
				src := c.read(it, f)
				gotest.Empty(it, focusPrefix.FindAllString(src, -1), "%s:\n%s", f, src)
				all += src
			}
			// The declaration, two receivers, a literal and two aliases.
			gotest.Len(it, word("ContractTestSuite").FindAllString(all, -1), 6, all)
			c.vet(it)
		})
	})
}

func (s *LinterFocusFixTestSuite) TestExternal(t *gotest.T) {
	t.When("lint -fix removes the focus of a suite the external test package refers to", func(w *gotest.T) {
		c := fixFocus(w, "lintfocusgeneric")
		w.It("lint -fix renames the external test package's references too", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			gotest.Equal(it, `package lintfocusgeneric_test

import "lintfocusmod/lintfocusgeneric"

var _ = lintfocusgeneric.ContractTestSuite[float64]{}
`, c.read(it, "contract_ext_test.go"))
		})
	})
}
