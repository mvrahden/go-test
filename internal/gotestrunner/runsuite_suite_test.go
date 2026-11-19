package gotestrunner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunSuiteTestSuite covers the one way a streaming run runs a suite, in each
// of its modes: every path books exactly one verdict.
// Sequential: it replaces the process-wide suite start.
type RunSuiteTestSuite struct{}

const runSuitePkg = "example.com/p"

// runSuitePlan plans XTestSuite, which reads p.A, and YTestSuite, which
// reads nothing.
func runSuitePlan() *gotestrunner.RunPlan {
	return gotestrunner.ExportPlanRun(gotestgen.GenerateResults{{
		PkgPath: runSuitePkg, PTest: []byte("x"),
		SuiteNames:                     []string{"XTestSuite", "YTestSuite"},
		SuiteRequiredSharedFixtureKeys: map[string][]string{"TestXTestSuite": {"p.A"}},
	}})
}

// runOne runs suite in mode on session f and returns the captured stream
// followed by the collector's stderr, the package's exit code and how many
// suite processes were started.
func runOne(t *gotest.T, f gotestrunner.ExportSession, suite string, mode gotestrunner.ExportSuiteMode, stop bool) (stream string, exit, starts int) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if stop {
		cancel()
	}
	var stdout, stderr bytes.Buffer
	c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&stdout, &stderr))
	c.Register(runSuitePkg, 1)
	restore := gotestrunner.ExportFailSuiteStarts(errors.New("must not start"), func() { starts++ })
	gotestrunner.ExportRunSuite(gotestrunner.ExportNewStreamRun(ctx, runSuitePlan(), c, f, 1), suite, mode)
	restore()
	c.Finalize()
	return string(c.CapturedJSON()) + stderr.String(), c.WorstExitCode(), starts
}

func (s *RunSuiteTestSuite) TestEveryPathBooksOneVerdict(t *gotest.T) {
	notAFile := filepath.Join(t.TempDir(), "file")
	gotest.NoError(t, os.WriteFile(notAFile, nil, 0o600))
	up := map[string]json.RawMessage{"p.A": json.RawMessage(`{}`)}
	modes := []struct {
		name string
		mode gotestrunner.ExportSuiteMode
	}{{"streamed", gotestrunner.ExportStreamed}, {"deferred", gotestrunner.ExportDeferred}, {"alone", gotestrunner.ExportAlone}}

	for _, m := range modes {
		t.When(m.name+", the run already stopped", func(w *gotest.T) {
			// A missing fixture as well: a stopped run says stopped.
			stream, _, starts := runOne(w, gotestrunner.ExportNewFixtureSession(nil, errors.New("setup failed"), false), "TestXTestSuite", m.mode, true)
			w.It("books the suite skipped as run stopping and starts nothing", func(it *gotest.T) {
				gotest.Contains(it, stream, `"Action":"skip"`)
				gotest.Contains(it, stream, "run stopping")
				gotest.NotContains(it, stream, "never ran")
				gotest.NotContains(it, stream, "TestXTestSuite/")
				gotest.Zero(it, starts)
			})
		})
		t.When(m.name+", its fixture never came up", func(w *gotest.T) {
			stream, exit, starts := runOne(w, gotestrunner.ExportNewFixtureSession(nil, errors.New("setup failed"), false), "TestXTestSuite", m.mode, false)
			w.It("books it failed, naming the fixture, and starts nothing", func(it *gotest.T) {
				gotest.Contains(it, stream, "TestXTestSuite never ran: shared fixture A did not come up")
				gotest.Contains(it, stream, `"Action":"fail"`)
				gotest.Equal(it, 1, exit)
				gotest.Zero(it, starts)
			})
		})
		t.When(m.name+", the state file cannot be written", func(w *gotest.T) {
			proc := gotestrunner.ExportNewUpProcess(filepath.Join(notAFile, "shared"), up)
			stream, exit, starts := runOne(w, gotestrunner.ExportNewFixtureSession(proc, nil, false), "TestXTestSuite", m.mode, false)
			w.It("fails the package and starts nothing", func(it *gotest.T) {
				gotest.Equal(it, 1, exit)
				gotest.NotContains(it, stream, "never ran")
				gotest.Zero(it, starts)
			})
		})
	}
}

func (s *RunSuiteTestSuite) TestASuiteWithNothingInItsWayStarts(t *gotest.T) {
	for _, mode := range []gotestrunner.ExportSuiteMode{gotestrunner.ExportStreamed, gotestrunner.ExportDeferred, gotestrunner.ExportAlone} {
		stream, exit, starts := runOne(t, gotestrunner.ExportNewFixtureSession(nil, nil, false), "TestYTestSuite", mode, false)
		gotest.Equal(t, 1, starts)
		gotest.NotEqual(t, 0, exit)
		gotest.Contains(t, stream, "did not start: must not start")
	}
}
