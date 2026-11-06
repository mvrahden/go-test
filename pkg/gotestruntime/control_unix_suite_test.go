//go:build unix

package gotestruntime_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envLinkMode set is the suite process of a
// scenario below, started with the runner's channel.
const (
	envLinkMode  = "GOTESTRUNTIME_LINK_MODE"  // what its holding method does
	envLinkDir   = "GOTESTRUNTIME_LINK_DIR"   // where it leaves its markers
	envLinkSlow  = "GOTESTRUNTIME_LINK_SLOW"  // how long its AfterAll takes
	envLinkGate  = "GOTESTRUNTIME_LINK_GATE"  // the file TestMain waits for before m.Run
	envLinkAfter = "GOTESTRUNTIME_LINK_AFTER" // what TestMain does once m.Run returned
)

// What the holding method does.
const (
	holdQuick      = "quick"      // returns at once
	holdOnContext  = "hold"       // returns once its context ends
	holdIgnoring   = "ignore"     // sees its context end and runs on
	holdGrandchild = "grandchild" // starts a process in its group, then holds
	holdStacks     = "stacks"     // writes every goroutine's stack and returns
)

// What TestMain does once m.Run returned.
const (
	afterLinger = "linger" // sends itself SIGTERM and lives on to say so
	afterStacks = "stacks" // writes every goroutine's stack
)

// markIn appends m to dir's markers; nothing without a dir.
func markIn(dir, m string) {
	if dir == "" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the scenario's own dir
	if err != nil {
		return
	}
	_, _ = f.WriteString(m + "\n")
	_ = f.Close()
}

func writeStacks(path string) {
	buf := make([]byte, 1<<20)
	_ = os.WriteFile(path, buf[:runtime.Stack(buf, true)], 0o600)
}

// awaitLinkGate holds a gated suite process before m.Run until its scenario
// opens the gate: what reaches the channel meanwhile comes while no suite is
// open.
func awaitLinkGate() {
	gate := os.Getenv(envLinkGate)
	if gate == "" {
		return
	}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(gate); err == nil {
			return
		}
	}
}

// afterLinkRun is a suite process's TestMain once its run ended, as a
// TestMain releasing what it set up would still be running then.
func afterLinkRun() {
	dir := os.Getenv(envLinkDir)
	switch os.Getenv(envLinkAfter) {
	case afterLinger:
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
		markIn(dir, "lingered")
	case afterStacks:
		writeStacks(filepath.Join(dir, "after"))
	}
}

// runLinkChild is the suite process's own suite, framed as a generated
// harness frames one: a fixture, a method that holds, one after it.
func runLinkChild(t *testing.T) {
	dir, mode := os.Getenv(envLinkDir), os.Getenv(envLinkMode)
	slow, _ := time.ParseDuration(os.Getenv(envLinkSlow))
	mark := func(m string) { markIn(dir, m) }
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			Fixtures: func(tb testing.TB) {
				gotestruntime.HoldFixtures(tb, func() gotestruntime.MainConfig {
					return gotestruntime.MainConfig{MaxSuiteSetupTimeout: 30 * time.Second, Fixtures: []*gotestruntime.FixtureNode{{
						Name:      "HeldFixture",
						Config:    gotest.FixtureConfig{Timeout: time.Minute},
						BeforeAll: func(context.Context) error { mark("fixture BeforeAll"); return nil },
						AfterAll:  func(context.Context) error { mark("fixture AfterAll"); return nil },
					}}}
				})
			},
			BeforeAll: func(*gotest.T) {},
			AfterAll: func(*gotest.T) {
				mark("AfterAll began")
				time.Sleep(slow)
				mark("suite AfterAll")
			},
		})
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{
			{Name: "TestHolds", Run: func(ttt *gotest.T, run func(func())) {
				run(func() { holdAs(ttt, mode, dir, mark) })
			}},
			{Name: "TestSecond", Run: func(ttt *gotest.T, run func(func())) {
				run(func() { mark("TestSecond ran") })
			}},
		}})
	})
}

func holdAs(t *gotest.T, mode, dir string, mark func(string)) {
	switch mode {
	case holdQuick:
		return
	case holdStacks:
		writeStacks(filepath.Join(dir, "during"))
		return
	case holdGrandchild:
		// Ignores SIGTERM, so only the group's SIGKILL ends it.
		sleeper := exec.Command("sh", "-c", "trap '' TERM; exec sleep 4242")
		if sleeper.Start() == nil {
			_ = os.WriteFile(filepath.Join(dir, "grandchild"), []byte(strconv.Itoa(sleeper.Process.Pid)), 0o600)
		}
	}
	mark("holding")
	<-t.Context().Done()
	mark("context ended")
	if mode == holdIgnoring {
		time.Sleep(time.Minute)
	}
}

// child is how a scenario starts its suite process.
type child struct {
	mode  string        // what the holding method does
	slow  time.Duration // how long AfterAll takes
	after string        // what TestMain does after m.Run
	gated bool          // TestMain waits for the gate before m.Run
	env   []string
}

// linkRun is one suite process of a scenario: this test binary, started as
// the runner starts one, with the channel and in a process group of its own,
// so the group it ends once the runner is gone is its own.
type linkRun struct {
	dir      string
	cmd      *exec.Cmd
	link     *control.Link
	exited   chan struct{}
	exitedAt time.Time
	read     chan struct{}

	mu    sync.Mutex
	lines []control.Line
}

// start runs suite's method in a suite process.
func (r *linkRun) start(t *gotest.T, suite, method string, c child) {
	r.dir = t.TempDir()
	env := []string{envLinkMode + "=" + c.mode, envLinkDir + "=" + r.dir, envLinkSlow + "=" + c.slow.String(), envLinkAfter + "=" + c.after}
	if c.gated {
		env = append(env, envLinkGate+"="+filepath.Join(r.dir, "gate"))
	}
	link, end, err := control.New()
	gotest.NoError(t, err)
	out, err := os.Create(filepath.Join(r.dir, "out"))
	gotest.NoError(t, err)
	cmd := exec.Command(os.Args[0], "-test.run=^"+suite+"$/^"+method+"$", "-test.v", "-test.count=1") //nolint:gosec // G204: this test binary
	cmd.Env = append(append(os.Environ(), env...), c.env...)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	end.Attach(cmd)
	err = cmd.Start()
	end.Close()
	_ = out.Close()
	if err != nil {
		link.Close()
	}
	gotest.NoError(t, err)
	r.cmd, r.link = cmd, link
	r.exited, r.read = make(chan struct{}), make(chan struct{})
	go func() {
		_ = cmd.Wait()
		r.exitedAt = time.Now()
		close(r.exited)
	}()
	go func() {
		defer close(r.read)
		for l := range link.Lines() {
			r.mu.Lock()
			r.lines = append(r.lines, l)
			r.mu.Unlock()
		}
	}()
}

func (r *linkRun) openGate(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(r.dir, "gate"), nil, 0o600))
}

// seen returns the lines of kind the suite process sent so far.
func (r *linkRun) seen(kind control.Kind) []control.Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	var lines []control.Line
	for _, l := range r.lines {
		if l.Kind == kind {
			lines = append(lines, l)
		}
	}
	return lines
}

// awaitLine waits for the first line of kind.
func (r *linkRun) awaitLine(t *gotest.T, kind control.Kind) control.Line {
	var line control.Line
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		lines := r.seen(kind)
		gotest.NotEmpty(poll, lines, "no %s line reached the runner", kind)
		line = lines[0]
	})
	return line
}

func (r *linkRun) markers() []string {
	data, _ := os.ReadFile(filepath.Join(r.dir, "markers"))
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (r *linkRun) awaitMarker(t *gotest.T, m string) {
	gotest.Eventually(t, 15*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.Contains(poll, r.markers(), m)
	})
}

func (r *linkRun) output() string {
	data, _ := os.ReadFile(filepath.Join(r.dir, "out"))
	return string(data)
}

// wait waits for the suite process to end and says how it did, as
// os.ProcessState spells it.
func (r *linkRun) wait(t *gotest.T) string {
	select {
	case <-r.exited:
		return r.cmd.ProcessState.String()
	case <-time.After(20 * time.Second):
		gotest.Fail(t, "the suite process did not end; its output:\n"+r.output())
		return ""
	}
}

// grandchild is the pid of the process the holding method started, or 0.
func (r *linkRun) grandchild() int {
	b, err := os.ReadFile(filepath.Join(r.dir, "grandchild"))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(b))
	return pid
}

// reap ends what the scenario left running: the suite process and its
// group, then the runner's end of the channel.
func (r *linkRun) reap() {
	if r.cmd == nil {
		return
	}
	select {
	case <-r.exited:
	default:
		_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL) // the group start gave it
		<-r.exited
	}
	r.link.Close()
	<-r.read
	if pid := r.grandchild(); pid > 0 && syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// ControlLinkTestSuite: a suite process the runner started with its channel
// takes the runner's stop, its force and its end on it, and tells the runner
// of a stop a signal began. Each method runs a suite process of its own.
type ControlLinkTestSuite struct{}

// SuiteConfig: a method took at most 1.6s locally under -race, 1s of it the
// race detector's exit; the default is far above twice that.
func (s *ControlLinkTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// BeforeAll, in a suite process, marks that its first suite opened.
func (s *ControlLinkTestSuite) BeforeAll(t *gotest.T) { markIn(os.Getenv(envLinkDir), "opened") }

// AfterAll has nothing to release.
func (s *ControlLinkTestSuite) AfterAll(t *gotest.T) {}

func (s *ControlLinkTestSuite) BeforeEach(t *gotest.T) *linkRun { return &linkRun{} }

func (s *ControlLinkTestSuite) AfterEach(t *gotest.T, r *linkRun) { r.reap() }

// suiteProcess runs a scenario's suite process side, reporting whether this
// is one.
func suiteProcess(t *gotest.T) bool {
	if os.Getenv(envLinkMode) == "" {
		return false
	}
	runLinkChild(t.T())
	return true
}

const linkSuite = "TestControlLinkTestSuite"

func (s *ControlLinkTestSuite) TestHelloComesBeforeTheFirstSuiteOpens(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestHelloComesBeforeTheFirstSuiteOpens", child{mode: holdQuick, gated: true})
	hello := r.awaitLine(t, control.Hello)
	early := r.markers()
	r.openGate(t)
	ended := r.wait(t)

	t.When("a suite process takes the runner's channel and its TestMain waits before the first suite opens", func(w *gotest.T) {
		w.It("says hello at once, with protocol version 1 and its pid", func(it *gotest.T) {
			gotest.Equal(it, control.Line{Kind: control.Hello, Version: 1, PID: r.cmd.Process.Pid}, hello)
			gotest.Empty(it, early, "a suite opened before the hello")
		})
		w.It("then opens its suite and ends with its tests' status", func(it *gotest.T) {
			gotest.Contains(it, r.markers(), "opened")
			gotest.Equal(it, "exit status 0", ended, r.output())
		})
	})
}

// beforeTheFirstOpen runs a gated suite process whose channel gets what
// send does while TestMain holds it, then lets it run.
func beforeTheFirstOpen(t *gotest.T, r *linkRun, method string, send func()) (ended string) {
	r.start(t, linkSuite, method, child{mode: holdQuick, gated: true})
	r.awaitLine(t, control.Hello)
	send()
	r.openGate(t)
	return r.wait(t)
}

func (s *ControlLinkTestSuite) TestAStopBeforeTheFirstOpenSkipsTheSuite(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	ended := beforeTheFirstOpen(t, r, "TestAStopBeforeTheFirstOpenSkipsTheSuite", func() {
		gotest.NoError(t, r.link.Stop())
	})

	t.When("the runner's stop reaches the channel while TestMain runs, before any suite opened", func(w *gotest.T) {
		w.It("skips the suite with the reason run stopping before it sets anything up", func(it *gotest.T) {
			gotest.Contains(it, r.output(), "--- SKIP: "+linkSuite+" ")
			gotest.Contains(it, r.output(), "run stopping")
			gotest.Empty(it, r.markers(), "the suite opened")
		})
		w.It("ends with its tests' status", func(it *gotest.T) {
			gotest.Equal(it, "exit status 0", ended, r.output())
		})
	})
}

func (s *ControlLinkTestSuite) TestTheChannelsEndBeforeTheFirstOpenSkipsTheSuite(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	ended := beforeTheFirstOpen(t, r, "TestTheChannelsEndBeforeTheFirstOpenSkipsTheSuite", func() { r.link.Close() })

	t.When("the runner's end of the channel closes while TestMain runs, before any suite opened", func(w *gotest.T) {
		w.It("skips the suite with the reason run stopping before it sets anything up", func(it *gotest.T) {
			gotest.Contains(it, r.output(), "--- SKIP: "+linkSuite+" ")
			gotest.Contains(it, r.output(), "run stopping")
			gotest.Empty(it, r.markers(), "the suite opened")
		})
		w.It("ends with its tests' status", func(it *gotest.T) {
			gotest.Equal(it, "exit status 0", ended, r.output())
		})
	})
}

func (s *ControlLinkTestSuite) TestAStopWhileAMethodHolds(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestAStopWhileAMethodHolds", child{mode: holdOnContext, after: afterLinger})
	r.awaitMarker(t, "holding")
	gotest.NoError(t, r.link.Stop())
	ended := r.wait(t)
	out := r.output()

	t.When("the runner's stop line reaches a suite process while a method holds", func(w *gotest.T) {
		w.It("ends the method's context and skips the method not started with the reason run stopping", func(it *gotest.T) {
			gotest.Contains(it, out, "--- SKIP: "+linkSuite+"/TestAStopWhileAMethodHolds/suite/TestSecond ")
			gotest.Contains(it, out, "run stopping")
		})
		w.It("runs AfterAll, then releases the fixture", func(it *gotest.T) {
			gotest.Equal(it, []string{"opened", "fixture BeforeAll", "holding", "context ended",
				"AfterAll began", "suite AfterAll", "fixture AfterAll", "lingered"}, r.markers())
		})
		w.It("ends with its tests' status, a SIGTERM after the stop settled changing nothing", func(it *gotest.T) {
			gotest.Equal(it, "exit status 0", ended, out)
		})
	})
}

func (s *ControlLinkTestSuite) TestAForceCutsAStoppedMethodShort(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestAForceCutsAStoppedMethodShort", child{mode: holdIgnoring})
	r.awaitMarker(t, "holding")
	gotest.NoError(t, r.link.Stop())
	r.awaitMarker(t, "context ended")
	forced := time.Now()
	gotest.NoError(t, r.link.Force())
	ended := r.wait(t)
	took := r.exitedAt.Sub(forced)

	t.When("the runner's force line reaches a suite process whose stopped method ignores its context", func(w *gotest.T) {
		w.It("releases the fixture without AfterAll at once, well inside the grace", func(it *gotest.T) {
			gotest.Contains(it, r.markers(), "fixture AfterAll")
			gotest.NotContains(it, r.markers(), "AfterAll began")
			gotest.Less(it, took, 2500*time.Millisecond)
		})
		w.It("says so, and ends by the signal", func(it *gotest.T) {
			gotest.Contains(it, r.output(), "the runner forced the stop")
			gotest.Equal(it, "signal: terminated", ended, r.output())
		})
	})
}

func (s *ControlLinkTestSuite) TestAForceWithNoStopUnderWayForcesOne(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestAForceWithNoStopUnderWayForcesOne", child{mode: holdOnContext, slow: 2 * time.Second})
	r.awaitMarker(t, "holding")
	gotest.NoError(t, r.link.Force())
	ended := r.wait(t)

	t.When("the runner's force line reaches a suite process with no stop under way", func(w *gotest.T) {
		// The method's context ends too, but the release does not wait for the
		// method to notice, so its marker may never be written.
		w.It("releases the fixture at once, without waiting for AfterAll", func(it *gotest.T) {
			gotest.Contains(it, r.markers(), "fixture AfterAll")
			gotest.NotContains(it, r.markers(), "suite AfterAll")
		})
		w.It("ends by the signal", func(it *gotest.T) {
			gotest.Equal(it, "signal: terminated", ended, r.output())
		})
	})
}

func (s *ControlLinkTestSuite) TestASignalFromOutsideIsAStopTheRunnerIsToldOf(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestASignalFromOutsideIsAStopTheRunnerIsToldOf", child{mode: holdOnContext, slow: time.Second})
	r.awaitMarker(t, "holding")
	gotest.NoError(t, syscall.Kill(r.cmd.Process.Pid, syscall.SIGTERM))
	stopped := r.awaitLine(t, control.Stopped)
	r.awaitMarker(t, "AfterAll began")
	gotest.NoError(t, syscall.Kill(r.cmd.Process.Pid, syscall.SIGTERM))
	ended := r.wait(t)

	t.When("SIGTERM from outside reaches a suite process on the runner's channel while no stop is under way", func(w *gotest.T) {
		w.It("tells the runner, once, that a SIGTERM stopped it", func(it *gotest.T) {
			gotest.Equal(it, "signal:SIGTERM", stopped.Reason)
			gotest.Len(it, r.seen(control.Stopped), 1)
		})
		w.It("drops a second SIGTERM during the stop: AfterAll runs, then the fixture is released", func(it *gotest.T) {
			gotest.Equal(it, []string{"opened", "fixture BeforeAll", "holding", "context ended",
				"AfterAll began", "suite AfterAll", "fixture AfterAll"}, r.markers())
		})
		w.It("ends by the signal once torn down", func(it *gotest.T) {
			gotest.Equal(it, "signal: terminated", ended, r.output())
		})
	})
}

func (s *ControlLinkTestSuite) TestTheBudgetLineCarriesWhatTheFileDoes(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	budgetFile := filepath.Join(t.TempDir(), "budget")
	r.start(t, linkSuite, "TestTheBudgetLineCarriesWhatTheFileDoes", child{mode: holdQuick,
		env: []string{protocol.EnvTeardownBudgetFile + "=" + budgetFile}})
	ended := r.wait(t)
	data, err := os.ReadFile(budgetFile) //nolint:gosec // G304: this method's own file
	gotest.NoError(t, err)
	inFile, err := time.ParseDuration(string(data))
	gotest.NoError(t, err)

	t.When("a suite process on the runner's channel sets its fixtures up", func(w *gotest.T) {
		w.It("sends the runner the teardown budget its budget file carries", func(it *gotest.T) {
			gotest.Equal(it, []control.Line{{Kind: control.Budget, Duration: inFile}}, r.seen(control.Budget))
			gotest.Equal(it, "exit status 0", ended, r.output())
		})
	})
}

// watcherFrame is what every goroutine of the stop watcher's runs in.
const watcherFrame = "github.com/mvrahden/go-test/pkg/gotestruntime.(*watcher)."

func (s *ControlLinkTestSuite) TestTheReaderEndsWithTheLastSuite(t *gotest.T, r *linkRun) {
	if suiteProcess(t) {
		return
	}
	r.start(t, linkSuite, "TestTheReaderEndsWithTheLastSuite", child{mode: holdStacks, after: afterStacks})
	ended := r.wait(t)
	during, err := os.ReadFile(filepath.Join(r.dir, "during"))
	gotest.NoError(t, err)
	after, err := os.ReadFile(filepath.Join(r.dir, "after"))
	gotest.NoError(t, err)

	t.When("a suite process on the runner's channel runs its suites and returns from m.Run", func(w *gotest.T) {
		w.It("reads the channel on one goroutine while a suite is open", func(it *gotest.T) {
			gotest.Equal(it, 1, strings.Count(string(during), watcherFrame+"read("), string(during))
		})
		w.It("leaves no goroutine of the stop watcher's once the last suite closed", func(it *gotest.T) {
			gotest.NotContains(it, string(after), watcherFrame)
			gotest.Equal(it, "exit status 0", ended, r.output())
		})
	})
}
