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

func (s *LinterDriversTestSuite) TestGoVet(t *gotest.T) {
	t.When("go vet runs a unitchecker main around the exported analyzer as its vettool", func(w *gotest.T) {
		w.It("go vet prints lint's findings for the package", func(it *gotest.T) {
			bin := filepath.Join(it.TempDir(), "lintvettool")
			if runtime.GOOS == "windows" {
				bin += ".exe"
			}
			build := exec.CommandContext(it.Context(), "go", "build", "-o", bin, "../testdata/lintvettool")
			out, err := build.CombinedOutput()
			gotest.NoError(it, err, "go build: %s", out)
			vet := exec.CommandContext(it.Context(), "go", "vet", "-vettool="+bin, "../testdata/lintergnolint/")
			out, err = vet.CombinedOutput()
			gotest.Error(it, err, "go vet passed: %s", out)
			gotest.Contains(it, string(out), "nolint_test.go:13:1: stdlib test TestForeign — consider using a gotest suite")
			gotest.Contains(it, string(out), "nolint_test.go:9:35: unknown rule not-a-rule in nolint comment")
		})
	})
}
