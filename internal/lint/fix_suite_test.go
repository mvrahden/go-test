package lint_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

// FixTestSuite holds Fix to the bytes the go/analysis driver's -fix writes
// when re-run until it converges, file for file.
// Sequential: Setenv(GOPATH, GO111MODULE, GOWORK).
type FixTestSuite struct {
	tmp    string
	driver string // the oracle: singlechecker over lint.Analyzer
}

func (s *FixTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *FixTestSuite) BeforeAll(t *gotest.T) {
	s.tmp = gotest.Must(os.MkdirTemp("", "lintfix"))
	s.driver = filepath.Join(s.tmp, "fixdriver")
	out, err := exec.Command("go", "build", "-o", s.driver, "./testdata/fixdriver").CombinedOutput() //nolint:gosec // G204: builds the test's own oracle
	gotest.NoError(t, err, string(out))
}

func (s *FixTestSuite) AfterAll(t *gotest.T) {
	gotest.NoError(t, os.RemoveAll(s.tmp))
}

// driverFix runs the oracle's -fix in dir until a run changes no file.
func (s *FixTestSuite) driverFix(t *gotest.T, dir string, patterns []string) {
	for range 10 {
		before := snapshot(t, dir)
		cmd := exec.Command(s.driver, append([]string{"-fix"}, patterns...)...) //nolint:gosec // G204: the test's own oracle binary
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		// Exit 1 is fixes held back; the next run applies them.
		var exitErr *exec.ExitError
		gotest.True(t, err == nil || errors.As(err, &exitErr) && exitErr.ExitCode() == 1, "driver: %v\n%s", err, out)
		if maps.Equal(before, snapshot(t, dir)) {
			return
		}
	}
	gotest.Fail(t, "the driver did not converge in 10 runs")
}

// engineFix runs Fix in dir and writes what it returns.
func engineFix(t *gotest.T, dir string, patterns []string) {
	targets := lint.Targets{Dir: dir, Patterns: patterns, Tests: true}
	res, err := lint.Analyze(t.Context(), targets)
	gotest.NoError(t, err)
	_, changes, err := lint.Fix(t.Context(), targets, res)
	gotest.NoError(t, err)
	for _, c := range changes {
		gotest.NoError(t, os.WriteFile(c.File, c.After, 0o600))
	}
}

// fixBoth copies src twice and fixes one copy with the driver, the other
// with Fix; env is set for each, with {root} standing for the copy's root.
func (s *FixTestSuite) fixBoth(t *gotest.T, src string, env map[string]string, dir string, patterns []string) (driver, engine string) {
	driver, engine = t.TempDir(), t.TempDir()
	for _, root := range []string{driver, engine} {
		copyTree(t, src, root)
		for k, v := range env {
			t.Setenv(k, strings.ReplaceAll(v, "{root}", root))
		}
		if root == driver {
			s.driverFix(t, filepath.Join(root, dir), patterns)
		} else {
			engineFix(t, filepath.Join(root, dir), patterns)
		}
	}
	return driver, engine
}

func (s *FixTestSuite) TestParity(t *gotest.T) {
	t.When("every rule's fixture package is fixed", func(w *gotest.T) {
		testdata := gotest.Must(filepath.Abs(analysistest.TestData()))
		var fixtures []string
		for _, e := range gotest.Must(os.ReadDir(filepath.Join(testdata, "src"))) {
			if e.IsDir() && e.Name() != "github.com" {
				fixtures = append(fixtures, "./"+e.Name())
			}
		}
		pristine := snapshot(w, testdata)
		env := map[string]string{"GOPATH": "{root}", "GO111MODULE": "off", "GOFLAGS": ""}
		driver, engine := s.fixBoth(w, testdata, env, "src", fixtures)

		w.It("writes what the driver writes", func(it *gotest.T) {
			gotest.Equal(it, snapshot(it, driver), snapshot(it, engine))
		})
		w.It("rewrites files in several fixtures", func(it *gotest.T) {
			changed := changedFiles(pristine, snapshot(it, engine))
			gotest.Contains(it, changed, "src/withsimplify/withsimplify_test.go")
			gotest.Contains(it, changed, "src/withfailguard/withfailguard_test.go")
			gotest.Contains(it, changed, "src/sample/sample_test.go")
		})
	})

	t.When("fixes overlap, drop imports and touch a generated file", func(w *gotest.T) {
		src := w.TempDir()
		repoRoot := gotest.Must(filepath.Abs("../.."))
		gotest.NoError(w, testkit.WriteModule(repoRoot, src, "fixprobe"))
		for name, body := range fixProbe {
			gotest.NoError(w, os.WriteFile(filepath.Join(src, name), []byte(body), 0o600))
		}
		driver, engine := s.fixBoth(w, src, map[string]string{"GOFLAGS": ""}, ".", []string{"./..."})
		got := snapshot(w, engine)

		w.It("writes what the driver writes", func(it *gotest.T) {
			gotest.Equal(it, snapshot(it, driver), got)
		})
		w.It("applies the overlapping fixes across passes", func(it *gotest.T) {
			gotest.Contains(it, got["probe_test.go"], `gotest.Equal(t, 3, x, "bad")`)
		})
		w.It("removes the imports the fixes left unused, named or shadowed", func(it *gotest.T) {
			gotest.NotContains(it, got["probe_test.go"], `"strings"`)
			gotest.NotContains(it, got["renamed_test.go"], `"strings"`)
		})
		w.It("keeps the imports still used", func(it *gotest.T) {
			gotest.Contains(it, got["probe_test.go"], `"errors"`)
			gotest.Contains(it, got["renamed_test.go"], `fmtx "fmt"`)
		})
		w.It("keeps an import whose name is not its last path element", func(it *gotest.T) {
			gotest.Contains(it, got["versioned_test.go"], `"math/rand/v2"`)
			gotest.Contains(it, got["versioned_test.go"], "gotest.Equal(t, 0, rand.IntN(1))")
		})
		w.It("leaves the generated file alone", func(it *gotest.T) {
			gotest.Equal(it, fixProbe["gen_test.go"], got["gen_test.go"])
		})
	})
}

// fixProbe is a module whose fixes overlap (fail-guard spans t-escape), drop
// imports (a named one, and one only a local shadows afterwards), keep an
// import named unlike its path, and include a generated file.
var fixProbe = map[string]string{
	"probe_test.go": `package fixprobe

import (
	"errors"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var errProbe = errors.New("probe")

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestOverlap(t *gotest.T) {
	x := 3
	if x != 3 {
		gotest.Fail(t.T(), "bad")
	}
}

func (s *ProbeTestSuite) TestImports(t *gotest.T) {
	gotest.True(t, strings.Contains("abc", "b"))
}

func (s *ProbeTestSuite) TestShadow(t *gotest.T) {
	strings := []string{"a"}
	gotest.Len(t, strings, 1)
}
`,
	"renamed_test.go": `package fixprobe

import (
	fmtx "fmt"
	strings "strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func (s *ProbeTestSuite) TestRenamed(t *gotest.T) {
	gotest.True(t, strings.Contains(fmtx.Sprint("abc"), "b"))
}
`,
	"versioned_test.go": `package fixprobe

import (
	"math/rand/v2"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func (s *ProbeTestSuite) TestVersioned(t *gotest.T) {
	gotest.True(t, rand.IntN(1) == 0)
}
`,
	"gen_test.go": `// Code generated by hand for the parity test. DO NOT EDIT.

package fixprobe

import "github.com/mvrahden/go-test/pkg/gotest"

func (s *ProbeTestSuite) TestGenerated(t *gotest.T) {
	gotest.True(t, 1 == 1)
}
`,
}

// snapshot maps every file beneath root, by slash path, to its contents.
func snapshot(t *gotest.T, root string) map[string]string {
	files := map[string]string{}
	fsys := os.DirFS(root)
	gotest.NoError(t, fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		files[path] = string(data)
		return err
	}))
	return files
}

// changedFiles names the files whose contents differ between two snapshots.
func changedFiles(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if before[k] != v {
			out = append(out, k)
		}
	}
	return out
}

func copyTree(t *gotest.T, src, dst string) {
	gotest.NoError(t, os.CopyFS(dst, os.DirFS(src)))
}
