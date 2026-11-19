package gotest_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeprecatedNamesTestSuite holds the constructors only generated code
// called: a harness an earlier gotest wrote still builds and runs against
// this tree, and the names are marked deprecated.
type DeprecatedNamesTestSuite struct {
	root string // the repository
	dir  string // the old harness's module, at a fixed path so the build cache keeps one entry
}

func (s *DeprecatedNamesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// BeforeAll clears what an interrupted run left in the module directory.
func (s *DeprecatedNamesTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	s.root = root
	s.dir = filepath.Join(root, ".scratch", "oldharness")
	gotest.NoError(t, os.RemoveAll(s.dir))
	gotest.NoError(t, os.MkdirAll(s.dir, 0o750))
}

func (s *DeprecatedNamesTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// goIn runs the go command in dir, outside any workspace, and returns its
// combined output and exit code (-1 when it did not start).
func goIn(t *gotest.T, dir string, args ...string) (string, int) {
	cmd := exec.CommandContext(t.Context(), "go", args...) //nolint:gosec // G204: go tool with test-controlled arguments
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil {
		return string(out) + err.Error(), -1
	}
	return string(out), cmd.ProcessState.ExitCode()
}

func (s *DeprecatedNamesTestSuite) TestOldHarness(t *gotest.T) {
	root, dir := s.root, s.dir
	src := filepath.Join("testdata", "oldharness")

	suite, err := os.ReadFile(filepath.Join(src, "old_test.go"))
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "old_test.go"), suite, 0o600))
	// Kept under a .txt name so the repository never holds a generated file.
	recorded, err := os.ReadFile(filepath.Join(src, "gotest_psuite_test.go.txt"))
	gotest.NoError(t, err)
	harness := string(recorded)
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "gotest_psuite_test.go"), recorded, 0o600))
	goMod := "module oldharness\n\ngo 1.25\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + root + "\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))

	tidy, code := goIn(t, dir, "mod", "tidy")
	gotest.Equal(t, 0, code, "go mod tidy: %s", tidy)
	out, exit := goIn(t, dir, "test", "-count=1", "-run", "^Fuzz", "-bench", ".", "-benchtime", "1x", ".")

	t.When("a harness written by the last release is compiled against this runtime", func(w *gotest.T) {
		w.It("builds, replays its fuzz seed and runs its benchmark", func(it *gotest.T) {
			gotest.Equal(it, 0, exit, "output: %s", out)
			gotest.Regexp(it, `(?m)^BenchmarkOldTestSuite/BenchmarkAdd\S*\s+1\s`, out)
			gotest.Regexp(it, `(?m)^ok\s`, out)
		})
		// A re-recorded harness that names the runtime would leave the run
		// above proving nothing.
		w.It("still names the user package's constructors", func(it *gotest.T) {
			gotest.Contains(it, harness, "ƒgotest.NewTFromTB(b)")
			gotest.Contains(it, harness, "ƒgotest.NewB(b)")
			gotest.Contains(it, harness, "ƒgotest.NewF(f, ")
		})
	})
}

func (s *DeprecatedNamesTestSuite) TestDocComments(t *gotest.T) {
	doc := func(t *gotest.T, name string) string {
		out, code := goIn(t, s.root, "doc", "./pkg/gotest", name)
		gotest.Equal(t, 0, code, "go doc %s: %s", name, out)
		return out
	}

	t.When("a name only generated code called is looked up", func(w *gotest.T) {
		w.It("is deprecated in favor of the runtime's", func(it *gotest.T) {
			for name, replacement := range map[string]string{
				"NewTFromTB": "EachT",
				"NewB":       "BenchB",
				"SeedArity":  "SeedArity",
				"SeedArg":    "SeedArg",
			} {
				gotest.Contains(it, doc(it, name), "Deprecated: generated code calls gotestruntime."+replacement)
			}
		})
	})
	t.When("a name users call is looked up", func(w *gotest.T) {
		w.It("is not deprecated", func(it *gotest.T) {
			for _, name := range []string{"NewF", "Record"} {
				gotest.NotContains(it, doc(it, name), "Deprecated:")
			}
		})
	})
}
