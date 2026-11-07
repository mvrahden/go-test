package lifecycle_test

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InterruptTestSuite stops a run while a fixture-bound test holds it open.
// Go's default for an interrupt ends a process on the spot, so the fixture is
// released only if the test binary takes the interrupt itself.
//
// Processes are started as the runner does it, through proctree and with
// the runner's channel, and stopped by its stop line.
type InterruptTestSuite struct {
	cli, binary, root string
}

func (s *InterruptTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: time.Minute}
}

func (s *InterruptTestSuite) BeforeAll(t *gotest.T) {
	sigintForChildren()
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	gotest.NoError(t, testkit.PrivateCache(t.TempDir()))
	s.root = stageTestdata(t, root)
	gen := exec.Command(s.cli, "generate", "./testdata/interrupt/") //nolint:gosec // G204: controlled binary with fixed args
	gen.Dir = s.root
	out, err := gen.CombinedOutput()
	gotest.NoError(t, err, string(out))
	name := "interrupt.test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	s.binary = filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "test", "-c", "-o", s.binary, "./testdata/interrupt/") //nolint:gosec // G204: fixed args
	build.Dir = s.root
	out, err = build.CombinedOutput()
	gotest.NoError(t, err, string(out))
}

// AfterAll has nothing to release: the staged module is a temp dir.
func (s *InterruptTestSuite) AfterAll(t *gotest.T) {}

// sigintForChildren makes the children this process starts begin with Go's
// default SIGINT. A process started with SIGINT ignored — as make runs its
// recipes — passes that on, and a stop by Ctrl-C would then never reach them;
// a handled signal is reset across exec. The runtime keeps an inherited
// ignore on purpose, as nohup intends.
var sigintOnce sync.Once

func sigintForChildren() {
	sigintOnce.Do(func() { signal.Notify(make(chan os.Signal, 1), os.Interrupt) })
}

// ownEnv is this process's environment without the runner's channel: a
// process started here answers to this suite, not to the runner.
func ownEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, control.Env+"=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// channel is the runner's end of a process's channel, with the lines the
// process sent on it.
type channel struct {
	link  *control.Link
	child *control.ChildEnd
	read  chan struct{}

	mu    sync.Mutex
	lines []control.Line
}

// attach gives cmd a channel, as the runner gives a suite process one; call
// started once cmd has started, and close once its tree is done.
func attach(t *gotest.T, cmd *exec.Cmd) *channel {
	link, child, err := control.New()
	gotest.NoError(t, err)
	child.Attach(cmd)
	c := &channel{link: link, child: child, read: make(chan struct{})}
	go func() {
		defer close(c.read)
		for l := range link.Lines() {
			c.mu.Lock()
			c.lines = append(c.lines, l)
			c.mu.Unlock()
		}
	}()
	return c
}

// started closes the end the process inherited, which the runner keeps no
// copy of.
func (c *channel) started() { c.child.Close() }

// close closes the runner's end, once the process's tree is done.
func (c *channel) close() {
	c.link.Close()
	<-c.read
}

// stopped returns the reasons of the stopped lines the process sent.
func (c *channel) stopped() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var reasons []string
	for _, l := range c.lines {
		if l.Kind == control.Stopped {
			reasons = append(reasons, l.Reason)
		}
	}
	return reasons
}

// startTree starts cmd as the root of its own process tree, logging to a fresh
// file, and returns once the test holds the fixture.
//
// The wait is a plain poll, not Eventually: control flow that leans on the
// assertions would let a drill mutant of the assertion engine interrupt a
// process that has not started, or never return. A tree that does not come up
// is killed before the test fails, so nothing outlives it; WaitDelay bounds a
// wait on output pipes a stray grandchild still holds.
func startTree(t *gotest.T, cmd *exec.Cmd) (tree *proctree.Tree, log string) {
	return startTreeUntil(t, cmd, "INTERRUPT_LOG", "running")
}

// startBound bounds a run that never logs its marker. The CLI compiles the
// package first, cold on a loaded runner, so the bound is generous.
const startBound = 5 * time.Minute

// startTreeUntil is startTree for a package that logs through logEnv and is
// ready once it logged marker.
func startTreeUntil(t *gotest.T, cmd *exec.Cmd, logEnv, marker string) (tree *proctree.Tree, log string) {
	log = filepath.Join(t.TempDir(), "events")
	cmd.Env = append(cmd.Env, logEnv+"="+log)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree = proctree.New(cmd)
	gotest.NoError(t, tree.Start())
	for deadline := time.Now().Add(startBound); !slices.Contains(readEvents(log), marker); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = wait(tree, cmd)
			gotest.Fail(t, "the process never logged %q:\n%s%s", marker, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // it must kill the tree before failing
		}
	}
	return tree, log
}

// wait waits for the tree's root and releases the tree.
func wait(tree *proctree.Tree, cmd *exec.Cmd) error {
	err := cmd.Wait()
	tree.Release()
	return err
}

// readEvents returns the lines a fixture package logged.
func readEvents(log string) []string {
	b, _ := os.ReadFile(log)
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// stopAsTheRunnerDoes stops the process the way the runner stops a suite
// that took its channel: by the stop line, with no signal. A line that cannot
// be sent kills the tree before the test fails, so nothing outlives it.
func stopAsTheRunnerDoes(t *gotest.T, c *channel, tree *proctree.Tree, cmd *exec.Cmd) {
	if err := c.link.Stop(); err != nil {
		_ = tree.Kill()
		_ = wait(tree, cmd)
		c.close()
		gotest.Fail(t, "the stop line was not sent: %v", err) //nolint:fail-guard // it must kill the tree before failing
	}
}

// The runner stops a suite process by a line on its channel; the process
// tears down and ends with its own status.
func (s *InterruptTestSuite) TestALineStopReleasesTheFixture(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = ownEnv()
	c := attach(t, cmd)
	tree, log := startTree(t, cmd)
	c.started()

	stopAsTheRunnerDoes(t, c, tree, cmd)
	err := wait(tree, cmd)
	c.close()

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.True(t, endedOnItsOwn(err), "the process ends with its own status, not by the signal: %v", err)
	gotest.Empty(t, c.stopped(), "the runner's own stop is not one from outside")
}

// A signal the runner did not send, such as code under test signalling its
// own process, is a first stop all the same: the process tears down, tells
// the runner, and ends by the signal.
func (s *InterruptTestSuite) TestASignalFromOutsideIsAStop(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = ownEnv()
	c := attach(t, cmd)
	tree, log := startTree(t, cmd)
	c.started()

	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)
	c.close()

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.False(t, endedOnItsOwn(err), "the process ends by the signal: %v", err)
	gotest.Equal(t, []string{"signal:" + interruptName}, c.stopped())
}

// The runner announces the stop and waits for the suite process to release
// its fixtures, on the text and the -json pipelines alike.
func (s *InterruptTestSuite) TestTheRunnerWaitsForTheRelease(t *gotest.T) {
	for _, flags := range [][]string{nil, {"-json"}} {
		cmd := exec.Command(s.cli, append(flags, "-run", "TestHoldingTestSuite", "./testdata/interrupt/")...) //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = s.root
		cmd.Env = ownEnv()
		tree, log := startTree(t, cmd)

		gotest.NoError(t, tree.Interrupt())
		_ = wait(tree, cmd)

		gotest.Equal(t, 130, cmd.ProcessState.ExitCode(), "gotest %v", flags)
		gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log), "gotest %v: released before the CLI exited", flags)
	}
}

// fuzzingWorkers waits until a worker is fuzzing beside the coordinator: each
// process logs "running" once it executes the target. The engine owns Ctrl-C
// only from then on; a worker signalled before it answered the coordinator's
// first ping is reported as having terminated without fuzzing.
func fuzzingWorkers(t *gotest.T, tree *proctree.Tree, cmd *exec.Cmd, log string) {
	for deadline := time.Now().Add(30 * time.Second); countEvents(readEvents(log), "running") < 2; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = wait(tree, cmd)
			gotest.Fail(t, "no worker started fuzzing:\n%s%s", cmd.Stdout, cmd.Stderr) //nolint:fail-guard // it must kill the tree before failing
		}
	}
}

func countEvents(events []string, kind string) int {
	n := 0
	for _, e := range events {
		if e == kind {
			n++
		}
	}
	return n
}

// Go stops fuzzing on Ctrl-C by itself: it stops the workers, saves what it
// found and the target returns, and every process tears its fixtures down
// after that. Nothing may cut in first.
func (s *InterruptTestSuite) TestCtrlCEndsFuzzingGracefully(t *gotest.T) {
	cmd := exec.Command(s.binary, "-test.run=^$", "-test.fuzz=FuzzFuzzingTestSuite_FuzzSpins", "-test.parallel=2", "-test.fuzztime=60s", "-test.fuzzcachedir="+t.TempDir()) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "INTERRUPT_FUZZ=1")
	tree, log := startTree(t, cmd)
	fuzzingWorkers(t, tree, cmd, log)
	tree.InterruptLikeCtrlC()

	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)

	gotest.NoError(t, err, "fuzzing ends cleanly on Ctrl-C:\n%s%s", cmd.Stdout, cmd.Stderr)
	gotest.Contains(t, cmd.Stdout.(*strings.Builder).String(), "PASS")
	events := readEvents(log)
	gotest.Equal(t, countEvents(events, "setup"), countEvents(events, "teardown"), "every fuzzing process released its fixture: %v", events)
	gotest.GreaterOrEqual(t, countEvents(events, "setup"), 2, "the coordinator and at least one worker")
}

// gotest fuzz interrupts its fuzzing processes the same way, so they end
// gracefully and release their fixtures before the CLI exits.
func (s *InterruptTestSuite) TestStoppingGotestFuzzReleasesTheFixture(t *gotest.T) {
	cmd := exec.Command(s.cli, "fuzz", "--target", "FuzzFuzzingTestSuite_FuzzSpins", "./testdata/interrupt/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.root
	cmd.Env = append(ownEnv(), "INTERRUPT_FUZZ=1")
	tree, log := startTree(t, cmd)
	fuzzingWorkers(t, tree, cmd, log)

	gotest.NoError(t, tree.Interrupt())
	_ = wait(tree, cmd)

	events := readEvents(log)
	gotest.Equal(t, countEvents(events, "setup"), countEvents(events, "teardown"), "every fuzzing process released its fixture: %v", events)
	gotest.GreaterOrEqual(t, countEvents(events, "setup"), 1)
}
