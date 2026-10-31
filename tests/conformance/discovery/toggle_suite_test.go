package discovery_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryToggleFocusTestSuite: refactor toggle-focus renames every
// reference to a suite in its package, and nothing that only shares its
// name.
type DiscoveryToggleFocusTestSuite struct{}

func (s *DiscoveryToggleFocusTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
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

// vet type-checks the staged module, test files included, built with tags.
func (c *toggled) vet(t *gotest.T, tags ...string) {
	cmd := exec.CommandContext(t.Context(), "go", "vet", "-tags="+strings.Join(tags, ","), "./...") //nolint:gosec // G204: build tags the suite passes
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
			c.vet(it)
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

// unchanged reports whether every file of the staged copy of the shapes
// package pkg still holds its testdata content.
func (c *toggled) unchanged(t *gotest.T, pkg string) {
	src := filepath.Join("..", "testdata", "shapes", pkg)
	entries, err := os.ReadDir(src)
	gotest.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		before, err := os.ReadFile(filepath.Join(src, e.Name()))
		gotest.NoError(t, err)
		gotest.Equal(t, string(before), c.read(t, e.Name()), "file %s", e.Name())
	}
}

func (s *DiscoveryToggleFocusTestSuite) TestEmbedded(t *gotest.T) {
	t.When("focus is toggled on a suite another type embeds", func(w *gotest.T) {
		c := toggle(w, "toggleembedded", "suite_test.go", "BaseTestSuite")
		w.It("refactor toggle-focus renames the selectors and literal keys that name the embedded field, and the package still compiles", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			gotest.Equal(it, `package toggleembedded

type wrapper struct{ F_BaseTestSuite }

func (w wrapper) count() int { return w.F_BaseTestSuite.n }

var _ = wrapper{F_BaseTestSuite: F_BaseTestSuite{n: 1}}.count
`, c.read(it, "wrapper_test.go"))
			c.vet(it)
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestExcludedFiles(t *gotest.T) {
	t.When("focus is toggled on a suite that files behind a build constraint refer to", func(w *gotest.T) {
		c := toggle(w, "toggletagged", "suite_test.go", "TaggedTestSuite")
		w.It("refactor toggle-focus renames their references too, and the package still compiles with the tag", func(it *gotest.T) {
			gotest.Equal(it, 0, c.run.Exit, "stderr: %s", c.run.Stderr)
			for _, f := range []string{"integration_test.go", "integration_ext_test.go"} {
				src := c.read(it, f)
				gotest.Empty(it, unfocused("TaggedTestSuite").FindAllString(src, -1), "%s:\n%s", f, src)
				gotest.Contains(it, src, "F_TaggedTestSuite", f)
			}
			c.vet(it)
			c.vet(it, "integration")
		})
		u := toggle(w, "toggleunresolved", "suite_test.go", "UnresolvedTestSuite")
		w.It("a reference in such a file that cannot be resolved is refused at its position and nothing is written", func(it *gotest.T) {
			gotest.NotEqual(it, 0, u.run.Exit)
			gotest.Regexp(it, `gen\.go:6:6: .*UnresolvedTestSuite`, u.run.Stderr)
			u.unchanged(it, "toggleunresolved")
			u.vet(it)
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestInterfaceMethod(t *gotest.T) {
	t.When("focus is toggled on a method an interface in the package names", func(w *gotest.T) {
		c := toggle(w, "toggleiface", "suite_test.go", "IfaceTestSuite.TestA")
		w.It("refactor toggle-focus refuses at the interface's position and writes nothing", func(it *gotest.T) {
			gotest.NotEqual(it, 0, c.run.Exit)
			gotest.Regexp(it, `assert_test\.go:5:7: .*interface.*TestA`, c.run.Stderr)
			c.unchanged(it, "toggleiface")
			c.vet(it)
		})
	})
}

func (s *DiscoveryToggleFocusTestSuite) TestNameTaken(t *gotest.T) {
	t.When("focus is toggled on a suite or method whose new name is already declared", func(w *gotest.T) {
		suite := toggle(w, "togglecollision", "suite_test.go", "CollisionTestSuite")
		method := toggle(w, "togglecollision", "suite_test.go", "CollisionTestSuite.TestA")
		w.It("refactor toggle-focus refuses at the declaration that holds the name and writes nothing", func(it *gotest.T) {
			for at, c := range map[string]*toggled{"taken_test.go:5:6": suite, "taken_test.go:9:30": method} {
				gotest.NotEqual(it, 0, c.run.Exit, at)
				gotest.Contains(it, c.run.Stderr, at)
				c.unchanged(it, "togglecollision")
				c.vet(it)
			}
		})
	})
}
