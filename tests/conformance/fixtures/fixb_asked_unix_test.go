//go:build unix

package fixtures_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AskedToStopTestSuite: the shared fixture process stops on a stop line and
// ends its releases' contexts on a force line, which a run's first and
// second Ctrl-C send it.
// Exclusive: a verdict times the second signal against the wall clock.
// Sequential: every method drives a live CLI the clock judges.
type AskedToStopTestSuite struct {
	cli string
}

// SuiteConfig: locally a method took 3s, the run reaching its method most of
// it; BeforeAll builds the CLI, 8.5s cold.
func (s *AskedToStopTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *AskedToStopTestSuite) BeforeAll(t *gotest.T) {
	s.cli = buildKillableCLI(t)
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *AskedToStopTestSuite) AfterAll(t *gotest.T) {}

// askedRun is a CLI run over one suite of fixbstop, stopped from outside.
type askedRun struct {
	cmd            *exec.Cmd
	tree           *proctree.Tree
	stdout, stderr strings.Builder
	markers        string
	exited         chan struct{}
	exitedAt       time.Time
}

// startAsked starts the CLI over suite with env and returns once its method
// holds; a run that never gets there is killed before the test fails.
func startAsked(t *gotest.T, cli, suite string, env ...string) *askedRun {
	markerDir := t.TempDir()
	r := &askedRun{markers: filepath.Join(markerDir, testkit.MarkersFile), exited: make(chan struct{})}
	r.cmd = exec.Command(cli, "-json", "../testdata/fixbstop/", "-run=^Test"+suite+"$") //nolint:gosec // G204: binary built by this suite
	r.cmd.Env = testkit.ChildEnv(markerDir, env...)
	r.cmd.Stdout, r.cmd.Stderr = &r.stdout, &r.stderr
	tree, err := testkit.StartCLI(r.cmd)
	gotest.NoError(t, err)
	r.tree = tree
	go func() {
		_ = r.cmd.Wait()
		r.exitedAt = time.Now()
		close(r.exited)
	}()
	r.await(t, "suite holds")
	return r
}

// await returns once marker is stamped; a run that ends first, or never
// stamps it, is killed before the test fails.
func (r *askedRun) await(t *gotest.T, marker string) {
	for deadline := time.Now().Add(testkit.Scaled(90 * time.Second)); ; time.Sleep(10 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(r.markers)
		if slices.Contains(got, marker) {
			return
		}
		select {
		case <-r.exited:
		default:
			if time.Now().Before(deadline) {
				continue
			}
		}
		r.ended(0)
		gotest.Fail(t, "the run never reached %q:\n%s%s", marker, &r.stdout, &r.stderr) //nolint:fail-guard // the tree is killed before the test fails
	}
}

// interrupt sends the CLI a SIGINT and returns when; a run it does not reach
// is killed before the test fails.
func (r *askedRun) interrupt(t *gotest.T) time.Time {
	sent := time.Now()
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		r.ended(0)
		gotest.Fail(t, "signal: %v", err) //nolint:fail-guard // the tree is killed before the test fails
	}
	return sent
}

// ended waits up to within for the CLI to exit, kills it when it has not,
// and returns its exit code, -2 for a run killed here, what it wrote and the
// markers.
func (r *askedRun) ended(within time.Duration) (exit int, out string, markers []string) {
	select {
	case <-r.exited:
		exit = r.cmd.ProcessState.ExitCode()
	case <-time.After(within):
		_ = r.tree.Kill()
		<-r.exited
		exit = -2
	}
	r.tree.Release()
	markers, _ = testkit.ReadMarkers(r.markers)
	return exit, r.stdout.String() + r.stderr.String(), markers
}

// fixturePID is the pid the shared fixture marked as "pid=<n>", 0 for none.
func fixturePID(markers []string) int {
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, "pid="); ok {
			pid, _ := strconv.Atoi(v)
			return pid
		}
	}
	return 0
}

// signalsIn are the stop signals the shared fixture marked it got.
func signalsIn(markers []string) []string {
	var got []string
	for _, m := range markers {
		if strings.HasPrefix(m, "signal ") {
			got = append(got, m)
		}
	}
	return got
}

// endedOnItsOwn reports whether the shared fixture process has exited
// within 2s; one still running is killed.
func endedOnItsOwn(pid int) bool {
	if pid <= 0 {
		return false
	}
	if testkit.EndedWithin(pid, testkit.Scaled(2*time.Second)) {
		return true
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return false
}

// killNotes are what the run says when it killed the shared fixture process.
var killNotes = []string{"forcing termination", "died during teardown", "force-killed", "exited before teardown was requested"}

func (s *AskedToStopTestSuite) TestAStoppedRun(t *gotest.T) {
	r := startAsked(t, s.cli, "HoldTestSuite")
	r.interrupt(t)
	exit, out, markers := r.ended(testkit.Scaled(20 * time.Second))
	pid := fixturePID(markers)

	t.When("a run is interrupted while a suite bound to a shared fixture holds", func(w *gotest.T) {
		w.It("the fixture's AfterAll runs and its process exits on its own", func(it *gotest.T) {
			gotest.Equal(it, 130, exit, out)
			gotest.Contains(it, markers, "AfterAll ends", out)
			for _, note := range killNotes {
				gotest.NotContains(it, out, note)
			}
			gotest.True(it, endedOnItsOwn(pid), "the shared fixture process %d outlived the run; markers: %v", pid, markers)
		})
		w.It("no signal reaches the fixture process", func(it *gotest.T) {
			gotest.NotZero(it, pid, "markers: %v", markers)
			gotest.Empty(it, signalsIn(markers), "markers: %v", markers)
		})
	})
}

func (s *AskedToStopTestSuite) TestASecondCtrlCDuringTheRelease(t *gotest.T) {
	r := startAsked(t, s.cli, "HoldTestSuite", "FIXB_RELEASE=wait")
	first := r.interrupt(t)
	r.await(t, "AfterAll begins")
	// Past the window in which a repeat is the first delivered again.
	time.Sleep(time.Until(first.Add(time.Second)))
	sent := r.interrupt(t)
	exit, out, markers := r.ended(testkit.Scaled(10 * time.Second))
	took := r.exitedAt.Sub(sent)
	pid := fixturePID(markers)

	t.When("a second SIGINT reaches gotest while a shared fixture's AfterAll waits on its context for 20s", func(w *gotest.T) {
		w.It("the AfterAll's context ends and it returns, and gotest exits 130 within 1.5s", func(it *gotest.T) {
			gotest.Equal(it, 130, exit, out)
			gotest.True(it, testkit.MarkerOrderOf(markers, "AfterAll begins", "AfterAll context ended", "AfterAll ends"), "markers: %v", markers)
			gotest.Less(it, took, testkit.Scaled(1500*time.Millisecond))
		})
		w.It("no process is killed and none is left behind", func(it *gotest.T) {
			for _, note := range killNotes {
				gotest.NotContains(it, out, note)
			}
			gotest.True(it, endedOnItsOwn(pid), "the shared fixture process %d outlived the run; markers: %v", pid, markers)
		})
	})
}

func (s *AskedToStopTestSuite) TestASecondCtrlCWhileTheSuiteStillRuns(t *gotest.T) {
	r := startAsked(t, s.cli, "IgnoreTestSuite", "FIXB_RELEASE=wait")
	first := r.interrupt(t)
	time.Sleep(time.Until(first.Add(time.Second)))
	// The suite still runs, so nothing has asked the fixture to stop yet, and
	// it outlasts the second SIGINT, so nothing does but the force line.
	before, _ := testkit.ReadMarkers(r.markers)
	r.interrupt(t)
	exit, out, markers := r.ended(testkit.Scaled(10 * time.Second))
	pid := fixturePID(markers)

	t.When("a second SIGINT reaches gotest while a suite bound to a shared fixture ignores the first and its package fixture takes 10s to release", func(w *gotest.T) {
		w.It("the fixture's AfterAll runs with its context ended, and its process exits on its own", func(it *gotest.T) {
			gotest.NotContains(it, before, "AfterAll begins", "the fixture was released before the second SIGINT")
			gotest.Equal(it, 130, exit, out)
			gotest.True(it, testkit.MarkerOrderOf(markers, "AfterAll begins", "AfterAll context ended", "AfterAll ends"), "markers: %v", markers)
			for _, note := range killNotes {
				gotest.NotContains(it, out, note)
			}
			gotest.True(it, endedOnItsOwn(pid), "the shared fixture process %d outlived the run; markers: %v", pid, markers)
		})
	})
}
