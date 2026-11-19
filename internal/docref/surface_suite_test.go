package docref_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SurfaceTestSuite holds docref's flag and environment tables to the code:
// a flag or a GOTEST_ variable the code adds fails here until it is described.
type SurfaceTestSuite struct{}

func (s *SurfaceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var root = filepath.Join("..", "..")

// registeredFlags reads the keys of gotest's flag registry from its source:
// the registry lives in package main, which no test can import.
func registeredFlags(t *gotest.T) map[string]bool {
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "cmd", "gotest", "flags.go"), nil, parser.SkipObjectResolution)
	gotest.NoError(t, err)
	flags := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "gotestFlags" || len(spec.Values) != 1 {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		gotest.True(t, ok, "gotestFlags in cmd/gotest/flags.go is not a map literal")
		for _, elt := range lit.Elts {
			key := elt.(*ast.KeyValueExpr).Key.(*ast.BasicLit)
			name, err := strconv.Unquote(key.Value)
			gotest.NoError(t, err)
			flags[name] = true
		}
		return false
	})
	gotest.NotEmpty(t, flags, "cmd/gotest/flags.go declares no gotestFlags; point internal/docref's surface suite at the registry")
	return flags
}

func (s *SurfaceTestSuite) TestFlags(t *gotest.T) {
	registered := registeredFlags(t)
	described := map[string]bool{}
	for _, f := range docref.FlagDocs {
		described[f.Name] = true
	}
	t.When("the flag registry is compared with internal/docref", func(w *gotest.T) {
		w.It("describes every registered flag", func(it *gotest.T) {
			for name := range registered {
				gotest.True(it, described[name], "flag %q has no description in internal/docref", name)
			}
		})
		w.It("describes no flag the registry lacks", func(it *gotest.T) {
			for name := range described {
				gotest.True(it, registered[name], "internal/docref describes unknown flag %q", name)
			}
		})
		w.It("describes each flag once", func(it *gotest.T) {
			gotest.Len(it, described, len(docref.FlagDocs))
		})
	})
	t.It("renders the flags as a table", func(it *gotest.T) {
		got, err := docref.FlagSections()["flags/gotest"]("table")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^\\| Flag \\| Effect \\|\n\\|---\\|---\\|\n", got)
		gotest.Contains(it, got, "| `--min=<pct>` | ")
	})
	t.It("renders the flags as HTML table rows, code spans as code", func(it *gotest.T) {
		got, err := docref.FlagSections()["flags/gotest"]("html")
		gotest.NoError(it, err)
		gotest.Contains(it, got, "<tr><td><code>--min=&lt;pct&gt;</code></td><td>")
		gotest.Contains(it, got, "<code>F_</code>")
	})
}

func (s *SurfaceTestSuite) TestEnvironment(t *gotest.T) {
	t.It("names every environment variable the code reads", func(it *gotest.T) {
		gotest.NoError(it, docref.CheckEnv(filepath.Join(root, "cmd"), filepath.Join(root, "internal"), filepath.Join(root, "pkg")))
	})
	t.It("renders the environment as a table", func(it *gotest.T) {
		got, err := docref.EnvSections()["env/user"]("table")
		gotest.NoError(it, err)
		gotest.Contains(it, got, "| `GOTEST_CI` | ")
	})
	t.It("renders the environment as HTML table rows", func(it *gotest.T) {
		got, err := docref.EnvSections()["env/user"]("html")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^<tr><td><code>GOTEST_CI</code></td><td>", got)
	})
}
