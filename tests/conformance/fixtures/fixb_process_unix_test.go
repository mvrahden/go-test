//go:build unix

package fixtures_test

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedProcessTestSuite: what a shared fixture process hands a process its
// setup starts.
type SharedProcessTestSuite struct{}

func (s *SharedProcessTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SharedProcessTestSuite) TestAProcessTheSetupStarts(t *gotest.T) {
	markerDir := t.TempDir()
	run, err := testkit.RunPackage(t.Context(), markerDir, "../testdata/fixbdetached/")
	ended := time.Now()
	gotest.NoError(t, err)
	// Timed from the started process's first write, which it outlives by
	// 30s, so the builds before it are not counted.
	var took time.Duration
	if ready, err := os.Stat(filepath.Join(markerDir, "detached-ready")); err == nil {
		took = ended.Sub(ready.ModTime())
	}
	t.When("a shared fixture's setup starts a process that outlives it", func(w *gotest.T) {
		w.It("the run passes before that process ends", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestDetachTestSuite/TestStateArrived"))
			gotest.NotZero(it, took, "the started process never wrote")
			gotest.Less(it, took, 25*time.Second, "the run waited for the 30s process")
		})
		w.It("the process sees no protocol variable, and what it writes to descriptor 3 is not read", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "env=")
			gotest.NotContains(it, run.Stdout+run.Stderr, "written by a process the fixture started")
		})
	})
	if data, err := os.ReadFile(filepath.Join(markerDir, "detached-pid")); err == nil {
		if pid, err := strconv.Atoi(string(data)); err == nil {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}
