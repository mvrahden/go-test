package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopGraceTestSuite: how long a stopped method's body may take to return,
// what that bound covers, and where a stop ends an async wait.
type StopGraceTestSuite struct {
	cli string
}

func (s *StopGraceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *StopGraceTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *StopGraceTestSuite) AfterAll(t *gotest.T) {}

// stopped runs the lcastop package with args and env, interrupts it once a
// method marked "running", and reads back what the run left. The marker
// "interrupted" is written just before the signal. A run that never holds,
// or that outlives the stop by 40s, is killed before the test fails.
func (s *StopGraceTestSuite) stopped(t *gotest.T, env []string, args ...string) (testkit.Run, time.Duration) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, append([]string{"-json", "../testdata/lcastop/"}, args...)...) //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	kill := func(format string, args ...any) {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		gotest.Fail(t, append([]any{format}, args...)...) //nolint:fail-guard // the tree is killed before the test fails
	}
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "running") {
			break
		}
		if time.Now().After(deadline) {
			kill("the run never held:\n%s%s", cmd.Stdout, cmd.Stderr)
		}
	}
	f, err := os.OpenFile(markers, os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = f.WriteString("interrupted\n")
		_ = f.Close()
	}
	began := time.Now()
	if err := testkit.Interrupt(cmd, tree); err != nil {
		kill("signal: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-exited:
	case <-time.After(40 * time.Second):
		kill("the run was still going 40s after the stop:\n%s%s", cmd.Stdout, cmd.Stderr)
	}
	took := time.Since(began)
	tree.Release()
	run := testkit.Run{Exit: cmd.ProcessState.ExitCode()}
	if waitErr != nil && run.Exit == 0 {
		run.Exit = -1
	}
	run.Stdout = cmd.Stdout.(*strings.Builder).String()
	run.Stderr = cmd.Stderr.(*strings.Builder).String()
	run.Events = testkit.ParseEvents([]byte(run.Stdout))
	run.Markers, _ = testkit.ReadMarkers(markers)
	return run, took
}

func (s *StopGraceTestSuite) TestABodyIgnoringItsContext(t *gotest.T) {
	run, took := s.stopped(t, []string{"LCA_IGNORE=1"}, "-run=TestGraceTestSuite")
	t.When("the run is stopped and a method body ignores its context", func(w *gotest.T) {
		w.It("cuts the body short after 5s, saying so", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "a method did not return within 5s of its context ending; it is cut short")
			gotest.GreaterOrEqual(it, took, 5*time.Second)
			gotest.Less(it, took, 20*time.Second)
			gotest.NotContains(it, run.Markers, "body returned")
		})
		w.It("releases the fixture without running AfterAll", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "fixture.AfterAll")
			gotest.NotContains(it, run.Markers, "suite.AfterAll")
		})
	})
}

func (s *StopGraceTestSuite) TestASlowAfterEach(t *gotest.T) {
	run, _ := s.stopped(t, []string{"LCA_SLOW_AFTEREACH=1"}, "-run=TestGraceTestSuite")
	t.When("the run is stopped and a method's AfterEach outlasts the 5s grace its body gets", func(w *gotest.T) {
		w.It("runs AfterEach to its end, then AfterAll, then releases the fixture: the grace bounds only the body", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "AfterEach began", "AfterEach ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
			gotest.NotContains(it, run.Stdout+run.Stderr, "cut short")
		})
	})
}

func (s *StopGraceTestSuite) TestAnAsyncWaitAfterAFailure(t *gotest.T) {
	run, _ := s.stopped(t, nil, "-run=TestAsyncHoldTestSuite")
	t.When("the run is stopped while an async method waits for done after another of its goroutines failed", func(w *gotest.T) {
		w.It("ends the wait at the stop, then runs AfterEach and AfterAll, and fails the method", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("interrupted", "async AfterEach", "async AfterAll"), "markers: %v", run.Markers)
			gotest.Equal(it, "fail", run.Verdict("TestAsyncHoldTestSuite/TestWaitsAsync"), "stdout: %s", run.Stdout)
		})
	})
}
