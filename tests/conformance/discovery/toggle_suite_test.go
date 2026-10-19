package discovery_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryToggleFocusTestSuite: refactor toggle-focus renames every
// reference to a suite in its package, and nothing that only shares its
// name.
type DiscoveryToggleFocusTestSuite struct{}

func (s *DiscoveryToggleFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// toggled is a staged copy of a shapes package after toggle-focus ran on it.
type toggled struct {
	mod, dir string
	run      testkit.Run
}

// toggle stages the shapes package pkg in module togglemod and toggles
// focus on ident, declared in file.
func toggle(t *gotest.T, pkg, file, ident string) *toggled {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	mod := t.TempDir()
	dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "shapes", pkg), mod, "togglemod")
	gotest.NoError(t, err)
	run, err := testkit.RunCLI(t.Context(), dir, "refactor", "toggle-focus", file, ident)
	gotest.NoError(t, err)
	return &toggled{mod: mod, dir: dir, run: run}
}

func (c *toggled) read(t *gotest.T, file string) string {
	src, err := os.ReadFile(filepath.Join(c.dir, file))
	gotest.NoError(t, err)
	return string(src)
}

// vet type-checks the staged module, test files included.
func (c *toggled) vet(t *gotest.T) {
	cmd := exec.CommandContext(t.Context(), "go", "vet", "./...")
	cmd.Dir = c.mod
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go vet: %s", out)
}

// unfocused matches name where it is not F_-prefixed.
func unfocused(name string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + name + `\b`)
}

func (s *DiscoveryToggleFocusTestSuite) TestSpread(t *gotest.T) {
	t.When("focus is toggled on a suite whose methods are spread over several files", func(w *gotest.T) {
		c := toggle(w, "togglespread", "suite_test.go", "SpreadTestSuite")
		w.It("refactor toggle-focus renames every receiver and the package still compiles", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			var all string
			for _, f := range []string{"suite_test.go", "more_test.go", "helpers_test.go"} {
				src := c.read(it, f)
				gotest.Empty(it, unfocused("SpreadTestSuite").FindAllString(src, -1), "%s:\n%s", f, src)
				all += src
			}
			// The declaration and four receivers.
			gotest.Equal(it, 5, strings.Count(all, "F_SpreadTestSuite"), all)
			c.vet(it)
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestGeneric(t *gotest.T) {
	t.When("focus is toggled on a generic suite that aliases instantiate", func(w *gotest.T) {
		c := toggle(w, "togglegeneric", "contract_test.go", "ContractTestSuite")
		w.It("refactor toggle-focus renames every generic receiver, alias and literal, and the package still compiles", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			var all string
			for _, f := range []string{"contract_test.go", "more_test.go", "aliases_test.go"} {
				src := c.read(it, f)
				gotest.Empty(it, unfocused("ContractTestSuite").FindAllString(src, -1), "%s:\n%s", f, src)
				all += src
			}
			// The declaration, two receivers, a literal and two aliases.
			gotest.Equal(it, 6, strings.Count(all, "F_ContractTestSuite"), all)
			c.vet(it)
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestExternal(t *gotest.T) {
	t.When("focus is toggled on a suite the external test package refers to", func(w *gotest.T) {
		c := toggle(w, "togglegeneric", "contract_test.go", "ContractTestSuite")
		w.It("refactor toggle-focus renames the external test package's references too", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			gotest.Equal(it, `package togglegeneric_test

import "togglemod/togglegeneric"

var _ = togglegeneric.F_ContractTestSuite[float64]{}
`, c.read(it, "contract_ext_test.go"))
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestReusedName(t *gotest.T) {
	t.When("focus is toggled on a suite whose name another scope reuses", func(w *gotest.T) {
		c := toggle(w, "toggleshadow", "suite_test.go", "ShadowTestSuite")
		w.It("refactor toggle-focus leaves the identifiers of the other scope as they were", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			before, err := os.ReadFile(filepath.Join("..", "testdata", "shapes", "toggleshadow", "others_test.go"))
			gotest.NoError(it, err)
			gotest.Equal(it, string(before), c.read(it, "others_test.go"))
			gotest.Equal(it, 2, strings.Count(c.read(it, "suite_test.go"), "F_ShadowTestSuite"))
			c.vet(it)
		})
	})
}
