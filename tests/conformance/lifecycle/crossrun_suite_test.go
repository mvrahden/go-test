package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CrossRunTestSuite: runs of one module take turns at their exclusive
// scopes. Each test starts runs of the crossrun packages that share one
// marker file and one gotest cache of their own; a run told to hold stays
// in its suite until the test creates its release file, so what overlaps
// is decided by the test, never by timing.
type CrossRunTestSuite struct {
	cli string
}

func (s *CrossRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *CrossRunTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *CrossRunTestSuite) AfterAll(t *gotest.T) {}

// sharedText is a writer a run's output goes to while the test reads it.
type sharedText struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *sharedText) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *sharedText) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// crossRun is one CLI run of a crossrun test.
type crossRun struct {
	cmd            *exec.Cmd
	tree           *proctree.Tree
	stdout, stderr *sharedText
	done           chan struct{} // closed once the run ended
	code           int
}

// crossRuns are the runs of one test: one marker dir, one cache.
type crossRuns struct {
	cli, dir, cache string
	runs            []*crossRun
}

func (s *CrossRunTestSuite) newRuns(t *gotest.T) *crossRuns {
	dir := t.TempDir()
	return &crossRuns{cli: s.cli, dir: dir, cache: filepath.Join(dir, "gotest-cache")}
}

// start runs the CLI as role on args, with env added.
func (c *crossRuns) start(t *gotest.T, role string, env []string, args ...string) *crossRun {
	cmd := exec.Command(c.cli, args...) //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(c.dir, append([]string{"GOTEST_CACHE_DIR=" + c.cache, "CROSSRUN_ROLE=" + role}, env...)...)
	r := &crossRun{cmd: cmd, stdout: new(sharedText), stderr: new(sharedText), done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = r.stdout, r.stderr
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	r.tree = tree
	c.runs = append(c.runs, r)
	gotest.Go(t, func() {
		_ = cmd.Wait()
		tree.Release()
		r.code = cmd.ProcessState.ExitCode()
		close(r.done)
	})
	return r
}

// release lets role's held suites end.
func (c *crossRuns) release(t *gotest.T, role string) {
	gotest.NoError(t, os.WriteFile(filepath.Join(c.dir, "release-"+role), nil, 0o600))
}

func (c *crossRuns) markers() []string {
	got, _ := testkit.ReadMarkers(filepath.Join(c.dir, testkit.MarkersFile))
	return got
}

// until returns once cond holds of the markers and the run's stderr, or
// fails the test, every run killed, after the budget.
func (c *crossRuns) until(t *gotest.T, what string, cond func(markers []string) bool) {
	for deadline := time.Now().Add(testkit.Scaled(90 * time.Second)); ; time.Sleep(20 * time.Millisecond) {
		if cond(c.markers()) {
			return
		}
		if time.Now().After(deadline) {
			c.kill()
			gotest.Fail(t, "never %s; markers: %v%s", what, c.markers(), c.outputs()) //nolint:fail-guard // every run is killed before the test fails
		}
	}
}

// waiting returns once r says it waits, r's role stamped begin, or r
// ended, whichever comes first; a run that does none of them within the
// budget is left to the test's checks.
func (c *crossRuns) waiting(r *crossRun, begins string) {
	for deadline := time.Now().Add(testkit.Scaled(60 * time.Second)); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		select {
		case <-r.done:
			return
		default:
		}
		if strings.Contains(r.stderr.String(), waits) || slices.Contains(c.markers(), begins) {
			return
		}
	}
}

// ended waits for r's exit code; a run that outlives the budget is killed
// and reads -1.
func (c *crossRuns) ended(r *crossRun, budget time.Duration) int {
	select {
	case <-r.done:
		return r.code
	case <-time.After(budget):
		_ = r.tree.Kill()
		<-r.done
		return -1
	}
}

// finish releases every role and waits for every run.
func (c *crossRuns) finish(t *gotest.T, roles ...string) {
	for _, role := range roles {
		c.release(t, role)
	}
	for _, r := range c.runs {
		c.ended(r, testkit.Scaled(60*time.Second))
	}
}

func (c *crossRuns) kill() {
	for _, r := range c.runs {
		_ = r.tree.Kill()
	}
}

func (c *crossRuns) outputs() string {
	var b strings.Builder
	for i, r := range c.runs {
		b.WriteString("\nrun " + strconv.Itoa(i) + " stderr:\n" + r.stderr.String())
	}
	return b.String()
}

// waits is what a wait line says, whichever scope waits.
const waits = " for another gotest run"

var holdEnv = []string{"CROSSRUN_HOLD=1"}

func (s *CrossRunTestSuite) TestTwoRunsOfOneModule(t *gotest.T) {
	c := s.newRuns(t)
	c.start(t, "a", holdEnv, "-json", "../testdata/crossrun/hold/", "-run", "TestHoldTestSuite")
	c.until(t, "saw run a hold", func(m []string) bool { return slices.Contains(m, "a hold begins") })
	b := c.start(t, "b", nil, "-json", "../testdata/crossrun/other/")
	c.waiting(b, "b other begins")
	beforeRelease := c.markers()
	c.finish(t, "a")
	bExit := c.ended(b, time.Second)
	markers := c.markers()

	t.When("a run reaches its Exclusive suites while another run of the module runs its own", func(w *gotest.T) {
		w.It("says once on stderr that it waits, naming the other run's pid, and nothing of it in the -json stream", func(it *gotest.T) {
			pid := strconv.Itoa(c.runs[0].cmd.Process.Pid)
			gotest.Equal(it, 1, strings.Count(b.stderr.String(), waits), "stderr: %s", b.stderr)
			gotest.Contains(it, b.stderr.String(), "gotest: the exclusive suites of github.com/mvrahden/go-test wait for another gotest run (pid "+pid+" in ")
			gotest.NotContains(it, b.stdout.String(), waits)
		})
		w.It("starts its Exclusive suite only once the other run's has ended", func(it *gotest.T) {
			gotest.Equal(it, 0, bExit, "stderr: %s", b.stderr)
			gotest.NotContains(it, beforeRelease, "b other begins")
			gotest.True(it, testkit.MarkerOrderOf(markers, "a hold ends", "b other begins"), "markers: %v", markers)
		})
	})
}

func (s *CrossRunTestSuite) TestARunWaitingForAGroup(t *gotest.T) {
	c := s.newRuns(t)
	c.start(t, "a", holdEnv, "-json", "../testdata/crossrun/hold/", "--group=e2e")
	c.until(t, "saw run a hold e2e", func(m []string) bool { return slices.Contains(m, "a e2e begins") })
	b := c.start(t, "b", nil, "-json", "../testdata/crossrun/hold/")
	c.waiting(b, "b e2e begins")
	third := c.start(t, "c", nil, "-json", "../testdata/crossrun/other/")
	thirdExit := c.ended(third, testkit.Scaled(30*time.Second))
	beforeRelease := c.markers()
	c.finish(t, "a")
	bExit := c.ended(b, time.Second)
	markers := c.markers()

	t.When("a run reaches group e2e while another run of the module runs that group", func(w *gotest.T) {
		w.It("runs its default group's Exclusive suite without waiting", func(it *gotest.T) {
			gotest.Contains(it, beforeRelease, "b hold ends")
			gotest.Contains(it, b.stderr.String(), "gotest: group e2e of github.com/mvrahden/go-test waits for another gotest run (")
		})
		w.It("holds no scope while it waits, so a third run's Exclusive suite runs at once", func(it *gotest.T) {
			gotest.Equal(it, 0, thirdExit, "stderr: %s", third.stderr)
			gotest.Contains(it, beforeRelease, "c other ends")
			gotest.NotContains(it, third.stderr.String(), waits)
		})
		w.It("starts its e2e suite only once the other run's has ended", func(it *gotest.T) {
			gotest.Equal(it, 0, bExit, "stderr: %s", b.stderr)
			gotest.NotContains(it, beforeRelease, "b e2e begins")
			gotest.True(it, testkit.MarkerOrderOf(markers, "a e2e ends", "b e2e begins"), "markers: %v", markers)
		})
	})
}

func (s *CrossRunTestSuite) TestStoppedWhileItWaits(t *gotest.T) {
	c := s.newRuns(t)
	c.start(t, "a", holdEnv, "-json", "../testdata/crossrun/hold/", "-run", "TestHoldTestSuite")
	c.until(t, "saw run a hold", func(m []string) bool { return slices.Contains(m, "a hold begins") })
	b := c.start(t, "b", nil, "-json", "../testdata/crossrun/other/")
	c.waiting(b, "b other begins")
	signalErr := testkit.Interrupt(b.cmd, b.tree)
	// Run a still holds: a run that waits on regardless never ends here.
	bExit := c.ended(b, testkit.Scaled(30*time.Second))
	c.finish(t, "a")
	run := testkit.Run{Stdout: b.stdout.String(), Events: testkit.ParseEvents([]byte(b.stdout.String()))}

	t.When("a run waiting for another run's exclusive scope is interrupted", func(w *gotest.T) {
		w.It("exits 130, skipping its Exclusive suite with the reason run stopping", func(it *gotest.T) {
			gotest.NoError(it, signalErr)
			gotest.Equal(it, 130, bExit, "stderr: %s", b.stderr)
			gotest.Equal(it, "skip", run.Verdict("TestOtherTestSuite"), "stream:\n%s", run.Stdout)
			gotest.Contains(it, outputOf(&run, "TestOtherTestSuite"), "run stopping")
			gotest.NotContains(it, c.markers(), "b other begins")
		})
	})
}

func (s *CrossRunTestSuite) TestItsTimeoutPassesWhileItWaits(t *gotest.T) {
	c := s.newRuns(t)
	c.start(t, "a", holdEnv, "-json", "../testdata/crossrun/hold/", "-run", "TestHoldTestSuite")
	c.until(t, "saw run a hold", func(m []string) bool { return slices.Contains(m, "a hold begins") })
	// The deadline outlasts the build of a package run a has linked.
	b := c.start(t, "b", nil, "--timeout="+testkit.Scaled(10*time.Second).String(), "../testdata/crossrun/hold/", "-run", "TestHoldTestSuite")
	bExit := c.ended(b, testkit.Scaled(60*time.Second))
	c.finish(t, "a")

	t.When("a run's --timeout passes while it waits for another run's exclusive scope", func(w *gotest.T) {
		w.It("fails, saying the timeout passed while its exclusive suites waited for another gotest run", func(it *gotest.T) {
			gotest.Equal(it, 1, bExit, "stderr: %s", b.stderr)
			gotest.Contains(it, b.stderr.String(), "while the exclusive suites of github.com/mvrahden/go-test waited for another gotest run")
			gotest.NotContains(it, c.markers(), "b hold begins")
		})
	})
}

func (s *CrossRunTestSuite) TestTheLockTurnedOff(t *gotest.T) {
	c := s.newRuns(t)
	c.start(t, "a", holdEnv, "-json", "../testdata/crossrun/hold/", "-run", "TestHoldTestSuite")
	c.until(t, "saw run a hold", func(m []string) bool { return slices.Contains(m, "a hold begins") })
	b := c.start(t, "b", []string{"GOTEST_EXCLUSIVE_LOCK=0"}, "-json", "../testdata/crossrun/other/")
	bExit := c.ended(b, testkit.Scaled(30*time.Second))
	markers := c.markers()
	c.finish(t, "a")

	t.When("a run with GOTEST_EXCLUSIVE_LOCK=0 reaches its Exclusive suites while another run of the module runs its own", func(w *gotest.T) {
		w.It("runs its Exclusive suite at once", func(it *gotest.T) {
			gotest.Equal(it, 0, bExit, "stderr: %s", b.stderr)
			gotest.Contains(it, markers, "b other ends")
			gotest.NotContains(it, markers, "a hold ends")
			gotest.NotContains(it, b.stderr.String(), waits)
		})
	})
}

func (s *CrossRunTestSuite) TestARunAnExclusiveSuiteStarts(t *gotest.T) {
	c := s.newRuns(t)
	parent := c.start(t, "parent", []string{"CROSSRUN_CLI=" + s.cli}, "-json", "../testdata/crossrun/nest/")
	exit := c.ended(parent, testkit.Scaled(90*time.Second))
	nested, _ := os.ReadFile(filepath.Join(c.dir, "nested-stderr"))
	markers := c.markers()

	t.When("an Exclusive suite runs gotest on an Exclusive suite of its module, under the same cache", func(w *gotest.T) {
		w.It("the nested run takes the scope its parent holds and passes without waiting", func(it *gotest.T) {
			gotest.Equal(it, 0, exit, "stdout: %s\nstderr: %s", parent.stdout, parent.stderr)
			gotest.Contains(it, markers, "nested hold ends")
			gotest.NotContains(it, string(nested), waits)
		})
	})
}
