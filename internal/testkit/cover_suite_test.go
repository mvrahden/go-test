package testkit_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLICoverageTestSuite: a CLI built while GOCOVERDIR is set is instrumented
// and keyed apart, so a run can measure what its CLI subprocesses execute.
type CLICoverageTestSuite struct{}

func (s *CLICoverageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 2 * time.Minute}
}

// stageKeyModule writes a module keymod whose cmd/gotest prints one line.
func stageKeyModule(t *gotest.T) string {
	mod := t.TempDir()
	main := filepath.Join(mod, "cmd", "gotest", "main.go")
	gotest.NoError(t, os.MkdirAll(filepath.Dir(main), 0o750))
	write(t, filepath.Join(mod, "go.mod"), "module keymod\n\ngo 1.26.0\n")
	write(t, main, "package main\n\nfunc main() { println(\"keymod\") }\n")
	return mod
}

func (s *CLICoverageTestSuite) TestACLIBuiltUnderGOCOVERDIRWritesItsCountersThere(t *gotest.T) {
	mod := stageKeyModule(t)
	t.Setenv("GOCOVERDIR", "")
	plainKey, err := testkit.ExportCLIKey(t.Context(), mod)
	gotest.NoError(t, err)

	t.When("GOCOVERDIR is not set", func(w *gotest.T) {
		binary, err := testkit.BuildCLI(w.Context(), mod, w.TempDir())
		gotest.NoError(w, err)
		out, err := exec.CommandContext(w.Context(), binary).CombinedOutput()
		w.It("builds the CLI as before: it runs without a word about coverage", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NotContains(it, string(out), "GOCOVERDIR")
		})
	})
	t.When("GOCOVERDIR is set", func(w *gotest.T) {
		data := w.TempDir()
		w.Setenv("GOCOVERDIR", data)
		binary, err := testkit.BuildCLI(w.Context(), mod, w.TempDir())
		gotest.NoError(w, err)
		gotest.NoError(w, exec.CommandContext(w.Context(), binary).Run())
		counters, _ := filepath.Glob(filepath.Join(data, "covcounters.*"))
		key, keyErr := testkit.ExportCLIKey(w.Context(), mod)
		w.It("the CLI writes its counters there when it exits", func(it *gotest.T) {
			gotest.NotEmpty(it, counters)
		})
		w.It("keys the build apart from a plain one", func(it *gotest.T) {
			gotest.NoError(it, keyErr)
			gotest.NotEqual(it, plainKey, key)
		})
	})
}
