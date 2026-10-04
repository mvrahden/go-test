package canary_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InterruptTestSuite stops a run while a fixture-bound test holds it open.
// Go's default for an interrupt ends a process on the spot, so the fixture is
// released only if the test binary takes the interrupt itself.
//
// Processes are started and stopped as the runner does it, through proctree:
// SIGTERM to the process group on Unix, CTRL_BREAK to the tree's console on
// Windows.
type InterruptTestSuite struct {
	cli, binary string
}

func (s *InterruptTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *InterruptTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	out, err := exec.Command(s.cli, "generate", "./testdata/interrupt/").CombinedOutput() //nolint:gosec // G204: controlled binary with fixed args
	gotest.NoError(t, err, string(out))
	name := "interrupt.test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	s.binary = filepath.Join(t.TempDir(), name)
	out, err = exec.Command("go", "test", "-c", "-o", s.binary, "./testdata/interrupt/").CombinedOutput() //nolint:gosec // G204: fixed args
	gotest.NoError(t, err, string(out))
}

func (s *InterruptTestSuite) AfterAll(t *gotest.T) {
	generated, _ := filepath.Glob("testdata/interrupt/gotest_p*suite_test.go")
	for _, f := range generated {
		_ = os.Remove(f)
	}
}

// ownEnv is this process's environment without the files the runner shares
// with it: a process started here answers to this suite, not to the runner.
func ownEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, protocol.EnvStopFile+"=") || strings.HasPrefix(kv, protocol.EnvTeardownBudgetFile+"=") {
			continue
		}
		env = append(env, kv)
	}
	return env
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
	log = filepath.Join(t.TempDir(), "events")
	cmd.Env = append(cmd.Env, "INTERRUPT_LOG="+log)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree = proctree.New(cmd)
	gotest.NoError(t, tree.Start())
	for deadline := time.Now().Add(30 * time.Second); !slices.Contains(readEvents(log), "running"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = wait(tree, cmd)
			gotest.Fail(t, "the process never held the fixture:\n%s%s", cmd.Stdout, cmd.Stderr) //nolint:fail-guard // it must kill the tree before failing
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

func readEvents(log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Fields(string(b))
}

// Under the runner an interrupt is a stop request only once the runner
// announced it; the runner creates the stop file before it interrupts.
func (s *InterruptTestSuite) TestAnAnnouncedStopReleasesTheFixture(t *gotest.T) {
	stop := filepath.Join(t.TempDir(), "stop")
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), protocol.EnvStopFile+"="+stop)
	tree, log := startTree(t, cmd)

	gotest.NoError(t, os.WriteFile(stop, nil, 0o600))
	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.True(t, stoppedFromOutside(err), "the process still ends as stopped from outside: %v", err)
}

// An interrupt the runner did not announce came from somewhere else, such as
// code under test signalling its own process; it is left to that code.
func (s *InterruptTestSuite) TestAnUnannouncedInterruptIsLeftAlone(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), protocol.EnvStopFile+"="+filepath.Join(t.TempDir(), "stop"))
	tree, log := startTree(t, cmd)

	gotest.NoError(t, tree.Interrupt())
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	gotest.Consistently(t, 500*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) {
		gotest.Empty(poll, exited, "the process ended on an interrupt nobody announced")
	})

	gotest.NoError(t, tree.Kill())
	<-exited
	tree.Release()
	gotest.Equal(t, []string{"setup", "running"}, readEvents(log))
}

// The runner announces the stop and waits for the suite process, including
// past test2json, which ends on the interrupt before the binary it runs.
func (s *InterruptTestSuite) TestTheRunnerWaitsForTheRelease(t *gotest.T) {
	for _, flags := range [][]string{nil, {"-json"}} {
		cmd := exec.Command(s.cli, append(flags, "-run", "TestHoldingTestSuite", "./testdata/interrupt/")...) //nolint:gosec // G204: controlled binary with fixed args
		cmd.Env = ownEnv()
		tree, log := startTree(t, cmd)

		gotest.NoError(t, tree.Interrupt())
		_ = wait(tree, cmd)

		gotest.Equal(t, 130, cmd.ProcessState.ExitCode(), "gotest %v", flags)
		gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log), "gotest %v: released before the CLI exited", flags)
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
// found and returns from m.Run, and every process tears its fixtures down
// after that. Nothing may cut in first.
func (s *InterruptTestSuite) TestCtrlCEndsFuzzingGracefully(t *gotest.T) {
	cmd := exec.Command(s.binary, "-test.run=^$", "-test.fuzz=FuzzFuzzingTestSuite_FuzzSpins", "-test.parallel=2", "-test.fuzztime=60s", "-test.fuzzcachedir="+t.TempDir()) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), "INTERRUPT_FUZZ=1")
	tree, log := startTree(t, cmd)
	tree.InterruptLikeCtrlC()

	gotest.NoError(t, tree.Interrupt())
	err := wait(tree, cmd)

	gotest.NoError(t, err, "fuzzing ends cleanly on Ctrl-C")
	gotest.Contains(t, cmd.Stdout.(*strings.Builder).String(), "PASS")
	events := readEvents(log)
	gotest.Equal(t, countEvents(events, "setup"), countEvents(events, "teardown"), "every fuzzing process released its fixture: %v", events)
	gotest.GreaterOrEqual(t, countEvents(events, "setup"), 2, "the coordinator and at least one worker")
}

// gotest fuzz interrupts its fuzzing processes the same way, so they end
// gracefully and release their fixtures before the CLI exits.
func (s *InterruptTestSuite) TestStoppingGotestFuzzReleasesTheFixture(t *gotest.T) {
	cmd := exec.Command(s.cli, "fuzz", "--target", "FuzzFuzzingTestSuite_FuzzSpins", "./testdata/interrupt/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = append(ownEnv(), "INTERRUPT_FUZZ=1")
	tree, log := startTree(t, cmd)

	gotest.NoError(t, tree.Interrupt())
	_ = wait(tree, cmd)

	events := readEvents(log)
	gotest.Equal(t, countEvents(events, "setup"), countEvents(events, "teardown"), "every fuzzing process released its fixture: %v", events)
	gotest.GreaterOrEqual(t, countEvents(events, "setup"), 1)
}
