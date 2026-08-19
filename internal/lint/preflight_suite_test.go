package lint_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PreflightTestSuite covers the compile gate in front of the analysis driver.
// The driver itself skips an uncompilable package and still exits 0 — a
// skipped analysis reported as a pass — so the gate is what makes a broken
// tree fail the lint step.
type PreflightTestSuite struct{}

// writeModule materializes a one-file module in dir; the load runs with that
// dir as its module root, so no chdir is needed and parallel suites stay
// undisturbed.
func writeModule(t *gotest.T, dir, source string) {
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module preflightprobe\n\ngo 1.25\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "probe.go"), []byte(source), 0o600))
}

func (s *PreflightTestSuite) TestUncompilablePackage(t *gotest.T) {
	t.When("a target package does not type-check", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		writeModule(w, dir, "package preflightprobe\n\nvar x int = \"nope\"\n")
		preflightErr := lint.PreflightLoad(dir, []string{"./..."})

		w.It("fails instead of letting the driver skip it as a pass", func(it *gotest.T) {
			gotest.ErrorContains(it, preflightErr, "cannot lint uncompilable packages")
			gotest.ErrorContains(it, preflightErr, "nope")
		})
	})

	t.When("the target package compiles", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		writeModule(w, dir, "package preflightprobe\n\nvar x = 1\n")

		w.It("passes", func(it *gotest.T) {
			gotest.NoError(it, lint.PreflightLoad(dir, []string{"./..."}))
		})
	})
}

func (s *PreflightTestSuite) TestPatternExtraction(t *gotest.T) {
	t.It("keeps patterns and drops flags", func(it *gotest.T) {
		got := lint.PreflightPatterns([]string{"-fix", "-skip-tescape", "./...", "./pkg/..."})
		gotest.Equal(it, []string{"./...", "./pkg/..."}, got)
	})
}
