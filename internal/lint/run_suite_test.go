package lint_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunTestSuite covers the programmatic analysis driver behind `gotest lint
// --github`, which needs findings as data instead of text on stderr.
// Not parallel: Setenv(GOWORK) is incompatible with parallel subtests.
type RunTestSuite struct{}

// writeProbeModule materializes a one-file module whose stdlib-style test
// triggers the migration-tier stdlib-test rule without importing gotest.
func writeProbeModule(t *gotest.T, dir, testSource string) {
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.24\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "probe_test.go"), []byte(testSource), 0o600))
}

func (s *RunTestSuite) TestRun(t *gotest.T) {
	t.When("a package violates a rule", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		writeProbeModule(w, dir, "package lintprobe\n\nimport \"testing\"\n\nfunc TestProbe(t *testing.T) {\n\tt.Log(\"probe\")\n}\n")
		findings, err := lint.Run(dir, []string{"./..."})

		w.It("returns the finding once despite test-variant package loads", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Len(it, findings, 1)
		})

		w.It("carries rule, position, and message", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Len(it, findings, 1)
			f := findings[0]
			gotest.Equal(it, string(lint.StdlibTest), f.Rule)
			gotest.True(it, strings.HasSuffix(f.File, "probe_test.go"), "file should be the probe test file, got %q", f.File)
			gotest.Greater(it, f.Line, 0)
			gotest.Greater(it, f.Col, 0)
			gotest.NotEmpty(it, f.Message)
		})
	})

	t.When("a package is clean", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lintprobe\n\ngo 1.24\n"), 0o600))
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "probe.go"), []byte("package lintprobe\n\nvar x = 1\n"), 0o600))
		findings, err := lint.Run(dir, []string{"./..."})

		w.It("returns no findings", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Empty(it, findings)
		})
	})
}
