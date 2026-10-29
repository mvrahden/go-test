package lifecycle_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GroupsTestSuite: a run's ExclusiveGroups are scopes that never overlap in
// time, each with its suites in parallel, then its exclusive ones.
// Exclusive: its verdicts read the wall clock.
type GroupsTestSuite struct {
	cli string
}

func (s *GroupsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 30 * time.Second, SetupTimeout: 45 * time.Second}
}

func (s *GroupsTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *GroupsTestSuite) AfterAll(t *gotest.T) {}

// parallelEnv lets the run start four suite processes whatever the
// machine's CPU count.
var parallelEnv = []string{"GOMAXPROCS=4"}

// stamped splits the groups package's "event@unixmilli" markers into the
// events in order and when each happened.
func stamped(markers []string) (events []string, at map[string]int64) {
	at = map[string]int64{}
	for _, m := range markers {
		event, ms, _ := strings.Cut(m, "@")
		events = append(events, event)
		at[event], _ = strconv.ParseInt(ms, 10, 64)
	}
	return events, at
}

// before reports whether every event of first comes before every event of then.
func before(events, first, then []string) bool {
	for _, a := range first {
		for _, b := range then {
			if !testkit.MarkerOrderOf(events, a, b) {
				return false
			}
		}
	}
	return true
}

func (s *GroupsTestSuite) TestGroupsRunApart(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), parallelEnv, "../testdata/groups/", "--parallel=4")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	events, at := stamped(run.Markers)

	t.When("suites of the default group, of a group e2e and of a later group run", func(w *gotest.T) {
		w.It("runs no suite of e2e beside a suite outside it", func(it *gotest.T) {
			e2eBegins := []string{"e2e.one begins", "e2e.two begins", "e2e.alone begins"}
			e2eEnds := []string{"e2e.one ends", "e2e.two ends", "e2e.alone ends"}
			gotest.True(it, before(events, []string{"default.a ends", "default.b ends"}, e2eBegins), "markers: %v", run.Markers)
			gotest.True(it, before(events, e2eEnds, []string{"load begins"}), "markers: %v", run.Markers)
		})
		w.It("runs the suites of e2e beside each other, two 1s suites within 1.8s", func(it *gotest.T) {
			gotest.True(it, before(events, []string{"e2e.one begins", "e2e.two begins"}, []string{"e2e.one ends", "e2e.two ends"}), "markers: %v", run.Markers)
			span := max(at["e2e.one ends"], at["e2e.two ends"]) - min(at["e2e.one begins"], at["e2e.two begins"])
			gotest.Less(it, span, int64(1800), "markers: %v", run.Markers)
		})
		w.It("runs e2e's Exclusive suite alone, after the group's other suites and before the next group", func(it *gotest.T) {
			gotest.True(it, before(events, []string{"e2e.one ends", "e2e.two ends"}, []string{"e2e.alone begins"}), "markers: %v", run.Markers)
			gotest.True(it, testkit.MarkerOrderOf(events, "e2e.alone ends", "load begins"), "markers: %v", run.Markers)
		})
	})
}

func (s *GroupsTestSuite) TestAGroupsFixtureIsResidentWithItsReaders(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), parallelEnv, "../testdata/groupfixtures/", "--parallel=4")
	gotest.NoError(t, err)
	t.When("a shared fixture only the e2e group reads, while another e2e suite outlasts its reader", func(w *gotest.T) {
		w.It("starts it after the default group ran, and releases it once its reader finished, while the group still runs", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("default ends", "stack.BeforeAll", "e2e stack ran", "stack.AfterAll", "e2e slow ends", "load.BeforeAll"), "markers: %v", run.Markers)
		})
	})
}

func (s *GroupsTestSuite) TestAStopDuringAGroup(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "../testdata/groupfixtures/", "--parallel=4") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, append(parallelEnv, "GROUPFIXTURES_HOLD=1")...)
	tree := reached(t, cmd, markerDir, "e2e stack holds")
	signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	run := finished(cmd, tree, markerDir)

	t.When("a run is stopped while a suite of the e2e group runs", func(w *gotest.T) {
		w.It("skips the later group's suites with the reason run stopping", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "skip", run.Verdict("TestLoadTestSuite"), "stream:\n%s", run.Stdout)
			gotest.Contains(it, outputOf(&run, "TestLoadTestSuite"), "run stopping")
			gotest.NotContains(it, run.Markers, "load ran")
		})
		w.It("starts none of the later group's fixtures, releases the group's and leaves no fixture process behind", func(it *gotest.T) {
			gotest.NotContains(it, run.Markers, "load.BeforeAll")
			gotest.Equal(it, 1, countOf(run.Markers, "stack.AfterAll"), "markers: %v\nstderr: %s", run.Markers, run.Stderr)
			pid := 0
			for _, m := range run.Markers {
				if n, ok := strings.CutPrefix(m, "setup pid "); ok {
					pid, _ = strconv.Atoi(n)
				}
			}
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, testkit.EndedWithin(pid, time.Second), "the shared fixture process %d still runs", pid)
		})
	})
}

// reached starts cmd as a CLI run whose streams are kept and returns once
// marker is stamped. A run that never stamps it is killed before the test
// fails, so nothing outlives it.
func reached(t *gotest.T, cmd *exec.Cmd, markerDir, marker string) *proctree.Tree {
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, marker) {
			return tree
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never reached %s:\n%s%s", marker, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

func countOf(markers []string, marker string) int {
	return len(slices.DeleteFunc(slices.Clone(markers), func(m string) bool { return m != marker }))
}
