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
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline}
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
	return s.stoppedRun(t, env, append([]string{"-json", "../testdata/lcastop/"}, args...), nil)
}

// stoppedRun is stopped over the CLI's whole argument list; then, when not
// nil, runs with the markers file right after the interrupt.
func (s *StopGraceTestSuite) stoppedRun(t *gotest.T, env, args []string, then func(markers string)) (testkit.Run, time.Duration) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, args...) //nolint:gosec // G204: binary built by this suite
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
	if f, err := os.OpenFile(markers, os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		_, _ = f.WriteString("interrupted\n")
		_ = f.Close()
	}
	began := time.Now()
	if err := testkit.Interrupt(cmd, tree); err != nil {
		kill("signal: %v", err)
	}
	if then != nil {
		then(markers)
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

func (s *StopGraceTestSuite) TestAStopDuringAnAsyncWait(t *gotest.T) {
	run, _ := s.stopped(t, nil, "-run=TestAsyncHoldTestSuite")
	waits, fails := "TestAsyncHoldTestSuite/TestWaitsAsync", "TestAsyncHoldTestSuite/TestFailsAsync"
	t.When("the run is stopped while an async method waits for done", func(w *gotest.T) {
		w.It("ends the wait at the stop and skips the method with the reason run stopping, then runs AfterEach and AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict(waits), run.Stdout)
			gotest.Contains(it, outputOf(&run, waits), "run stopping")
			gotest.True(it, run.MarkerOrder("interrupted", "async AfterEach "+waits, "async AfterAll"), "markers: %v", run.Markers)
		})
	})
	t.When("the run is stopped while an async method waits for done after another of its goroutines failed", func(w *gotest.T) {
		w.It("ends the wait at the stop, then runs AfterEach and AfterAll, and fails the method", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("interrupted", "async AfterEach "+fails, "async AfterAll"), "markers: %v", run.Markers)
			gotest.Equal(it, "fail", run.Verdict(fails), run.Stdout)
		})
	})
}

func (s *StopGraceTestSuite) TestABeforeAllIgnoringItsContext(t *gotest.T) {
	run, took := s.stopped(t, nil, "-run=TestSetupHangTestSuite")
	t.When("the run is stopped while a suite's BeforeAll ignores its context", func(w *gotest.T) {
		w.It("cuts BeforeAll short after 5s, saying so, and releases the fixture without running AfterAll", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "BeforeAll did not return within 5s of its context ending; it is cut short")
			gotest.GreaterOrEqual(it, took, 5*time.Second)
			gotest.Less(it, took, 20*time.Second)
			gotest.Contains(it, run.Markers, "fixture.AfterAll")
			gotest.NotContains(it, run.Markers, "suite.AfterAll")
			gotest.NotContains(it, run.Markers, "BeforeAll returned")
		})
	})
}

func (s *StopGraceTestSuite) TestABodyStartingAfterTheStop(t *gotest.T) {
	run, took := s.stopped(t, nil, "-run=TestLateBodyTestSuite")
	t.When("the run is stopped while a method's BeforeEach runs and its body then ignores its context", func(w *gotest.T) {
		w.It("cuts the body short 5s after it began, saying so", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "a method did not return within 5s of its context ending; it is cut short")
			gotest.Less(it, took, 20*time.Second)
			gotest.True(it, run.MarkerOrder("body began", "fixture.AfterAll"), "markers: %v", run.Markers)
			gotest.NotContains(it, run.Markers, "body returned")
		})
	})
}

func (s *StopGraceTestSuite) TestAContextAwareAfterEach(t *gotest.T) {
	run, _ := s.stopped(t, nil, "-run=TestCleanupTestSuite")
	t.When("the run is stopped and a method's AfterEach waits on its context", func(w *gotest.T) {
		w.It("leaves that context alone, so AfterEach runs to its end before AfterAll and the release", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("AfterEach ran to its end", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
			gotest.NotContains(it, run.Markers, "AfterEach context ended")
		})
	})
}

func (s *StopGraceTestSuite) TestAForcedStop(t *gotest.T) {
	run, _ := s.stopped(t, nil, "-run=TestStuckCleanupTestSuite")
	t.When("the run is stopped and an AfterEach ignores its context until only the fixtures' teardown time is left of the release bound", func(w *gotest.T) {
		w.It("forces the stop, saying so, and runs the fixture teardown to its end", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout+run.Stderr, "the run did not stop within its budget")
			gotest.True(it, run.MarkerOrder("release began", "release ended"), "markers: %v", run.Markers)
		})
	})
}

// stoppedBench runs the lcastopbench package as stopped does, with args after
// the package.
func (s *StopGraceTestSuite) stoppedBench(t *gotest.T, env []string, args ...string) (testkit.Run, time.Duration) {
	return s.stoppedRun(t, env, append([]string{"-json"}, append(args, "../testdata/lcastopbench/")...), nil)
}

func (s *StopGraceTestSuite) TestAStopDuringABenchmark(t *gotest.T) {
	run, _ := s.stoppedRun(t, nil, []string{"bench", "../testdata/lcastopbench/", "-args", "-test.benchtime=1x"}, nil)
	ignoring, took := s.stoppedRun(t, []string{"LCA_IGNORE=1"}, []string{"bench", "../testdata/lcastopbench/", "-args", "-test.benchtime=1x"}, nil)
	t.When("the run is stopped while a benchmark runs", func(w *gotest.T) {
		w.It("skips the benchmarks not started with the reason run stopping, then runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("first returned", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
			gotest.NotContains(it, run.Markers, "second ran")
			gotest.Contains(it, run.Stdout+run.Stderr, "run stopping")
		})
		w.It("cuts a benchmark that ignores the stop short after 5s, saying so, and releases the fixture", func(it *gotest.T) {
			gotest.Contains(it, ignoring.Stdout+ignoring.Stderr, "a benchmark did not return within 5s of its context ending; it is cut short")
			gotest.Less(it, took, 20*time.Second)
			gotest.Contains(it, ignoring.Markers, "fixture.AfterAll")
			gotest.NotContains(it, ignoring.Markers, "first returned")
		})
	})
}

func (s *StopGraceTestSuite) TestAStopDuringAFuzzSeed(t *gotest.T) {
	run, _ := s.stoppedBench(t, nil)
	t.When("the run is stopped while a fuzz seed replays", func(w *gotest.T) {
		w.It("skips the seeds not started with the reason run stopping, then runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("first seed returned", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
			gotest.NotContains(it, run.Markers, "second seed ran")
			gotest.Contains(it, run.Stdout, "run stopping")
		})
	})
}

func (s *StopGraceTestSuite) TestASecondStopSignal(t *gotest.T) {
	if !suiteSignallable {
		t.Skipf("only Unix signals a single process")
	}
	var sent error
	run, took := s.stoppedRun(t, []string{"LCA_IGNORE=1"}, []string{"-json", "../testdata/lcastop/", "-run=TestGraceTestSuite"}, func(markers string) {
		time.Sleep(500 * time.Millisecond)
		sent = signalSuite(markers)
	})
	t.When("a second stop signal reaches the suite while a stopped method body ignores its context", func(w *gotest.T) {
		w.It("releases the fixture at once, saying so, and the process ends by the signal", func(it *gotest.T) {
			gotest.NoError(it, sent)
			gotest.Contains(it, run.Stdout+run.Stderr, "a second stop signal")
			gotest.Contains(it, run.Markers, "fixture.AfterAll")
			gotest.Less(it, took, 4*time.Second)
		})
	})
}
