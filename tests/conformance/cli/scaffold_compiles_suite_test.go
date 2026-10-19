package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIScaffoldCompilesTestSuite: scaffold writes only code that compiles, and
// refuses a target whose scaffold would not.
type CLIScaffoldCompilesTestSuite struct{}

// SuiteConfig: the shape cases vet a module against this repository.
func (s *CLIScaffoldCompilesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// shapesSrc holds every target shape: a type with methods, an interface,
// functions to fuzz, and generic declarations.
const shapesSrc = `package shapes

// Calc is a type with methods.
type Calc struct{}

// Add returns a + b.
func (c *Calc) Add(a, b int) int { return a + b }

// Div divides, or fails on zero.
func (c *Calc) Div(a, b int) (int, error) { return a / b, nil }

// Store is an interface.
type Store interface {
	Get(key string) (string, error)
}

// Encode and Decode are inverses.
func Encode(s string) ([]byte, error) { return []byte(s), nil }

// Decode reverses Encode.
func Decode(b []byte) (string, error) { return string(b), nil }

// Render has no inverse.
func Render(n int) string { return "" }

// Index is a generic type.
type Index[T any] struct{ items []T }

// Len counts the items.
func (i *Index[T]) Len() int { return len(i.items) }

// Ident is a generic function.
func Ident[T any](v T) T { return v }
`

// stageShapes writes a module holding package shapes, plus extra files in
// it, and returns the module's directory and the package's.
func stageShapes(t *gotest.T, extra map[string]string) (modDir, pkgDir string) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	modDir = t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, modDir, "example.com/mod"))
	pkgDir = filepath.Join(modDir, "shapes")
	gotest.NoError(t, os.MkdirAll(pkgDir, 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(pkgDir, "shapes.go"), []byte(shapesSrc), 0o600))
	for name, src := range extra {
		gotest.NoError(t, os.WriteFile(filepath.Join(pkgDir, name), []byte(src), 0o600))
	}
	return modDir, pkgDir
}

// goFiles lists the .go files in dir.
func goFiles(t *gotest.T, dir string) []string {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	gotest.NoError(t, err)
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	return names
}

func (s *CLIScaffoldCompilesTestSuite) TestGenericTargets(t *gotest.T) {
	t.When("a generic type, a generic function or a method of a generic type is scaffolded", func(w *gotest.T) {
		w.It("refuses it on stderr with exit 1 and writes nothing", func(it *gotest.T) {
			for sub, tc := range gotest.Each(it, []struct {
				Desc string
				args []string
				want string
			}{
				{"a generic type", []string{"./shapes.Index"}, "Index is generic (type Index[T any])"},
				{"a generic function", []string{"--fuzz", "./shapes.Ident"}, "Ident is generic (func Ident[T any])"},
				{"a method of a generic type", []string{"./shapes.Index.Len"}, "Index.Len is a method of a generic type"},
			}) {
				dir, pkgDir := stageShapes(sub, nil)
				run, err := testkit.RunCLI(sub.Context(), dir, append([]string{"scaffold"}, tc.args...)...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, tc.want)
				gotest.Equal(sub, []string{"shapes.go"}, goFiles(sub, pkgDir), "scaffold wrote a file")
			}
		})
	})
}

func (s *CLIScaffoldCompilesTestSuite) TestTakenSuiteNames(t *gotest.T) {
	t.When("the package already declares the suite name scaffold would write", func(w *gotest.T) {
		w.It("refuses it on stderr with exit 1, naming the declaration and its file, and writes nothing", func(it *gotest.T) {
			for sub, tc := range gotest.Each(it, []struct {
				Desc string
				src  string
				want string
			}{
				{"a type in the package's test files", "package shapes\n\ntype CalcTestSuite struct{}\n", "CalcTestSuite is already declared: type CalcTestSuite in package shapes at "},
				{"a func in the external test package", "package shapes_test\n\nfunc CalcTestSuite() {}\n", "CalcTestSuite is already declared: func CalcTestSuite in package shapes_test at "},
			}) {
				dir, pkgDir := stageShapes(sub, map[string]string{"other_test.go": tc.src})
				run, err := testkit.RunCLI(sub.Context(), dir, "scaffold", "./shapes.Calc")
				gotest.NoError(sub, err)
				gotest.Equal(sub, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, tc.want+filepath.Join(pkgDir, "other_test.go")+":3")
				gotest.ElementsMatch(sub, []string{"shapes.go", "other_test.go"}, goFiles(sub, pkgDir), "scaffold wrote a file")
			}
		})
	})
}

func (s *CLIScaffoldCompilesTestSuite) TestTargetShapes(t *gotest.T) {
	t.When("each target shape is scaffolded", func(w *gotest.T) {
		w.It("writes a file go vet passes", func(it *gotest.T) {
			for sub, tc := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{"a type with methods", []string{"./shapes.Calc"}},
				{"an interface", []string{"./shapes.Store"}},
				{"a file of functions", []string{"./shapes/shapes.go"}},
				{"a function with an inverse", []string{"--fuzz", "./shapes.Encode"}},
				{"a function without one", []string{"--fuzz", "./shapes.Render"}},
			}) {
				dir, pkgDir := stageShapes(sub, nil)
				run, err := testkit.RunCLI(sub.Context(), dir, append([]string{"scaffold"}, tc.args...)...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Len(sub, goFiles(sub, pkgDir), 2, "scaffold wrote no file")
				vet := exec.CommandContext(sub.Context(), "go", "vet", ".")
				vet.Dir = pkgDir
				out, err := vet.CombinedOutput()
				gotest.NoError(sub, err, "go vet:\n%s", out)
			}
		})
	})
}
