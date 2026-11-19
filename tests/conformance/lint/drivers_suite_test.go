package lint_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterDriversTestSuite: the analyzer pkg/lint exports reports under other
// go/analysis drivers what gotest lint reports.
type LinterDriversTestSuite struct{}

func (s *LinterDriversTestSuite) SuiteConfig() gotest.SuiteConfig {
	// The It links x/tools' unitchecker, then vets a package with it.
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute}
}

// vettool builds the go vet wrapper around the exported analyzer.
func vettool(t *gotest.T) string {
	bin := filepath.Join(t.TempDir(), "lintvettool")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "../testdata/lintvettool")
	out, err := build.CombinedOutput()
	gotest.NoError(t, err, "go build: %s", out)
	return bin
}

// vet runs go vet with bin over pkg and returns what it printed.
func vet(t *gotest.T, bin, pkg string) (string, error) {
	out, err := exec.CommandContext(t.Context(), "go", "vet", "-vettool="+bin, pkg).CombinedOutput() //nolint:gosec // G204: the vet tool this test built
	return string(out), err
}

func (s *LinterDriversTestSuite) TestGoVet(t *gotest.T) {
	t.When("go vet runs a unitchecker main around the exported analyzer as its vettool", func(w *gotest.T) {
		w.It("go vet prints lint's findings for the package", func(it *gotest.T) {
			bin := vettool(it)
			out, err := vet(it, bin, "../testdata/lintergnolint/")
			gotest.Error(it, err, "go vet passed: %s", out)
			gotest.Contains(it, out, "nolint_test.go:13:1: stdlib test TestForeign — consider using a gotest suite")
			gotest.Contains(it, out, "nolint_test.go:9:35: unknown rule not-a-rule in nolint comment")
		})
	})
}

func (s *LinterDriversTestSuite) TestDroppedCasesUnderVet(t *gotest.T) {
	t.When("go vet runs the exported analyzer on directories with cases no suite runs", func(w *gotest.T) {
		bin := vettool(w)
		w.It("dropped-case reports nothing in a directory with an external test package, leaving it to the generator", func(it *gotest.T) {
			for _, pkg := range []string{"../testdata/lintdroppedext/", "../testdata/lintdroppedcomment/"} {
				out, _ := vet(it, bin, pkg)
				gotest.NotContains(it, out, "never runs", "%s", pkg)
			}
		})
		w.It("dropped-case reports the cases of a directory with no external test package", func(it *gotest.T) {
			out, err := vet(it, bin, "../testdata/lintdropped/")
			gotest.Error(it, err, "go vet passed: %s", out)
			gotest.Contains(it, out, "dropped_suite_test.go:8:1: TestCreate on UserSuite never runs")
			gotest.Contains(it, out, "dropped_suite_test.go:29:6: generic suite OrphanContractTestSuite never runs")
			gotest.Contains(it, out, "dropped_suite_test.go:72:1: TestAdded on Shared never runs")
			gotest.NotContains(it, out, "dropped_suite_test.go:43:")
		})
	})
}
