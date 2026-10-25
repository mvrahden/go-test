package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIEventsFileStopTestSuite: a run stopped or killed mid-way leaves its
// --events-file holding what it had written.
// Sequential: it builds the CLI once and every row drives a live process.
type CLIEventsFileStopTestSuite struct {
	cli string
}

func (s *CLIEventsFileStopTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: setup ~45s, the method ~45s for its three runs; the budgets are twice that.
	return gotest.SuiteConfig{Timeout: 90 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *CLIEventsFileStopTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *CLIEventsFileStopTestSuite) AfterAll(t *gotest.T) {}

// holding starts a run of the stopping package through cmd and returns once
// its first method holds. A run that never holds is killed before the test
// fails, so nothing outlives it.
func holding(t *gotest.T, cmd *exec.Cmd, markerDir string) *proctree.Tree {
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "running") {
			return tree
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never held:\n%s%s", cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

// signalled holds a signal's error; a run the signal did not reach is killed
// before the test fails, so nothing outlives it.
func signalled(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, err error) {
	if err == nil {
		return
	}
	_ = tree.Kill()
	_ = cmd.Wait()
	tree.Release()
	gotest.Fail(t, "signal: %v", err) //nolint:fail-guard // the tree is killed before the test fails
}

// finished waits for cmd and returns what it printed and its markers.
func finished(cmd *exec.Cmd, tree *proctree.Tree) string {
	_ = cmd.Wait()
	tree.Release()
	return cmd.Stdout.(*strings.Builder).String()
}

// passedPackage reports whether an events file holds the verdict of the
// passing package.
func passedPackage(file string) bool {
	for _, ev := range testkit.ParseEvents([]byte(file)) {
		if ev.Test == "" && ev.Action == "pass" && strings.HasSuffix(ev.Package, "/testdata/passing") {
			return true
		}
	}
	return false
}

// streamed waits until the events file holds the passing package's verdict:
// a suite's events reach it when the suite process ends. It gives up well
// before the holding method does, at its 30s default.
func streamed(path string) (string, bool) {
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := os.ReadFile(path)
		if passedPackage(string(got)) {
			return string(got), true
		}
		if time.Now().After(deadline) {
			return string(got), false
		}
	}
}

// incompleteLines are the lines of an events file that are not one whole
// JSON event ended by a newline.
func incompleteLines(file string) []string {
	var bad []string
	lines := strings.SplitAfter(file, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasSuffix(line, "\n") || !json.Valid([]byte(line)) {
			bad = append(bad, line)
		}
	}
	return bad
}

// stoppedRun is what a run stopped while a suite held left behind.
type stoppedRun struct {
	// before is the file once the passing package had ended; streamed says
	// whether it got there before the holding method gave up.
	before   string
	streamed bool
	// heldAtStop are the markers when the stop came.
	heldAtStop   []string
	file, stdout string
}

// stopWhileHolding runs the passing and the stopping package with
// --events-file and stops the run with stop once the passing package's
// events are in the file.
func (s *CLIEventsFileStopTestSuite) stopWhileHolding(t *gotest.T, stop func(cmd *exec.Cmd, tree *proctree.Tree) error) stoppedRun {
	markerDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "events.json")
	cmd := exec.Command(s.cli, "-json", "--events-file="+path, "../testdata/passing/", "../testdata/stopping/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := holding(t, cmd, markerDir)
	var r stoppedRun
	r.before, r.streamed = streamed(path)
	r.heldAtStop, _ = testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	if r.streamed {
		signalled(t, cmd, tree, stop(cmd, tree))
	} else {
		_ = tree.Kill()
	}
	r.stdout = finished(cmd, tree)
	got, err := os.ReadFile(path)
	gotest.NoError(t, err)
	r.file = string(got)
	return r
}

// keptWhatEnded checks that r's file holds what was in it before the stop,
// each line whole, as -json printed it.
func keptWhatEnded(it *gotest.T, r *stoppedRun) {
	gotest.True(it, r.streamed, "the events file never held the passing package before the stop:\n%s", r.before)
	gotest.NotContains(it, r.heldAtStop, "context ended", "the stop came after the holding method ended")
	gotest.Regexp(it, "^"+regexp.QuoteMeta(r.before), r.file)
	gotest.Empty(it, incompleteLines(r.file), "file:\n%s", r.file)
	gotest.LessOrEqual(it, len(r.file), len(r.stdout), "file:\n%s\nstdout:\n%s", r.file, r.stdout)
	gotest.Equal(it, r.file, r.stdout[:len(r.file)])
}

func (s *CLIEventsFileStopTestSuite) TestAStoppedRunLeavesItsEventsFile(t *gotest.T) {
	type row struct {
		Desc string
		stop func(cmd *exec.Cmd, tree *proctree.Tree) error
	}
	for t, row := range gotest.Each(t, []row{
		{Desc: "an interrupt", stop: func(cmd *exec.Cmd, tree *proctree.Tree) error {
			return testkit.Interrupt(cmd, tree)
		}},
		{Desc: "a forced stop", stop: func(cmd *exec.Cmd, tree *proctree.Tree) error {
			if err := testkit.Interrupt(cmd, tree); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			return testkit.Interrupt(cmd, tree)
		}},
	}) {
		r := s.stopWhileHolding(t, row.stop)

		t.When("a -json run with --events-file is stopped while a suite holds", func(w *gotest.T) {
			w.It("keeps the events of the suites that ended before the stop, each line whole", func(it *gotest.T) {
				keptWhatEnded(it, &r)
			})
			w.It("writes the stopped suite's events too, as -json prints them", func(it *gotest.T) {
				gotest.Contains(it, r.file, `"Test":"TestStopTestSuite/TestHolds"`)
				gotest.Equal(it, r.stdout, r.file)
			})
		})
	}
}

func (s *CLIEventsFileStopTestSuite) TestAKilledRunLeavesItsEventsFile(t *gotest.T) {
	r := s.stopWhileHolding(t, func(cmd *exec.Cmd, tree *proctree.Tree) error {
		err := cmd.Process.Kill()
		// The suite processes the CLI started are not part of the kill;
		// they go with the tree once the CLI is gone.
		time.Sleep(200 * time.Millisecond)
		_ = tree.Kill()
		return err
	})

	t.When("a -json run with --events-file is killed while a suite holds", func(w *gotest.T) {
		w.It("keeps the events of the suites that ended before the kill, each line whole", func(it *gotest.T) {
			keptWhatEnded(it, &r)
		})
	})
}
