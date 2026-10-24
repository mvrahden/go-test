package shutdown_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// ShutdownTestSuite proves that a run which ends early still tears its
// shared fixtures down: an interrupt exits 130 and a FailFast trip skips the
// remaining methods, and the fixture's AfterAll runs either way. Exclusive:
// it spawns full CLI runs and times a signal.
// Sequential: each test times a full CLI run against the wall clock.
//
//nolint:lifecycle-pair // BeforeAll's module lives under t.TempDir(), which the framework removes; the binary is the shared fixture's
type ShutdownTestSuite struct {
	CLI    *gotestcli.BinarySharedFixture
	binary string
	module string
}

func (s *ShutdownTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: slowest method ~38s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Exclusive: true, Timeout: 90 * time.Second}
}

func (s *ShutdownTestSuite) BeforeAll(t *gotest.T) {
	testkit.ScrubActionsEnv()
	s.binary = s.CLI.Binary
	s.module = t.TempDir()
	src := filepath.Join(s.CLI.RepoRoot, "tests", "sharedfixture", "testdata", "shutdownmod")
	gotest.NoError(t, testkit.StageModule(s.CLI.RepoRoot, src, s.module))
	// Compile the slow package once: the --timeout runs must not race a
	// cold build, which on a loaded runner outlasts them.
	warm := s.command(t.TempDir(), nil, []string{"-run=^$", "./slow/"})
	out, err := warm.CombinedOutput()
	gotest.NoError(t, err, string(out))
}

// cliRun is a CLI started as the root of its own process tree.
type cliRun struct {
	cmd  *exec.Cmd
	tree *proctree.Tree
}

// start launches the CLI over one package of the staged module with the
// marker directory in its environment.
func (s *ShutdownTestSuite) start(t *gotest.T, markers, pkg string) *cliRun {
	return s.startWith(t, markers, nil, "./"+pkg+"/")
}

// startWith launches the CLI with the given arguments and extra environment.
func (s *ShutdownTestSuite) startWith(t *gotest.T, markers string, env []string, args ...string) *cliRun {
	return s.launch(t, s.command(markers, env, args))
}

func (s *ShutdownTestSuite) command(markers string, env, args []string) *exec.Cmd {
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.module
	cmd.Env = append(append(os.Environ(), "GOTEST_SHUTDOWN_DIR="+markers, "GOTEST_CI=0"), env...)
	return cmd
}

// launch starts cmd as the root of its own process tree.
func (s *ShutdownTestSuite) launch(t *gotest.T, cmd *exec.Cmd) *cliRun {
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	return &cliRun{cmd: cmd, tree: tree}
}

// interruptWhen interrupts the run once the marker file appears.
func interruptWhen(t *gotest.T, run *cliRun, markers, name string) {
	gotest.Eventually(t, 90*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, marker(markers, name), "%s never appeared", name)
	})
	gotest.NoError(t, testkit.Interrupt(run.cmd, run.tree))
}

func exitCode(t *gotest.T, run *cliRun) int {
	err := run.cmd.Wait()
	run.tree.Release()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	gotest.True(t, errors.As(err, &exitErr), "wait: %v", err)
	return exitErr.ExitCode()
}

func marker(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func (s *ShutdownTestSuite) TestInterrupt(t *gotest.T) {
	markers := t.TempDir()
	run := s.start(t, markers, "slow")
	interruptWhen(t, run, markers, "test-running")
	code := exitCode(t, run)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-up"), "the fixture never started")
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

func (s *ShutdownTestSuite) TestFailFastTrip(t *gotest.T) {
	markers := t.TempDir()
	code := exitCode(t, s.start(t, markers, "failfast"))

	t.It("exits 1 and skips the methods after the failure", func(it *gotest.T) {
		gotest.Equal(it, 1, code)
		gotest.False(it, marker(markers, "second-ran"), "the method after the failure ran")
	})
	t.It("tears the shared fixture down", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

// The first interrupt starts the graceful shutdown; a second one must end
// the CLI at once, the way a terminal's Ctrl-C ends any other program, rather
// than being swallowed for the length of the teardown budget. Under spec the
// suite runs through go tool test2json, which ignores SIGINT, and the CLI
// inherits that: the exit must not depend on the default disposition.
func (s *ShutdownTestSuite) TestSecondInterruptEndsTheRun(t *gotest.T) {
	markers := t.TempDir()
	run := s.startWith(t, markers, []string{"GOTEST_SHUTDOWN_TEARDOWN_SLEEP=20s"}, "./slow/")
	interruptWhen(t, run, markers, "test-running")
	first := time.Now()
	gotest.Eventually(t, 90*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, marker(markers, "fixture-tearing-down"), "fixture-tearing-down never appeared")
	})
	// A repeat this soon after the first is the first delivered again, as
	// go tool forwards it: a person's second Ctrl-C comes later.
	time.Sleep(time.Until(first.Add(time.Second)))
	gotest.NoError(t, testkit.Interrupt(run.cmd, run.tree))
	sent := time.Now()

	exited := make(chan int, 1)
	go func() { exited <- exitCode(t, run) }()
	var code int
	select {
	case code = <-exited:
	case <-time.After(10 * time.Second):
		_ = run.tree.Kill()
		gotest.Fail(t, "the CLI was still running 10s after the second interrupt")
	}
	elapsed := time.Since(sent)

	data, err := os.ReadFile(filepath.Join(markers, "fixture-tearing-down"))
	gotest.NoError(t, err)
	setup, err := strconv.Atoi(string(data))
	gotest.NoError(t, err)

	t.It("exits 130 at once instead of waiting the teardown out", func(it *gotest.T) {
		gotest.Less(it, elapsed, 5*time.Second, "took %v", elapsed)
		gotest.Equal(it, 130, code)
	})
	t.It("leaves no shared fixture process behind", func(it *gotest.T) {
		gotest.True(it, testkit.EndedWithin(setup, 2*time.Second), "the shared fixture process %d outlived the CLI", setup)
	})
}

// A second interrupt while a method still ignores the first reaches the
// shared fixture before anything asked it to tear down: it is asked then,
// and its AfterAll runs before the CLI exits.
func (s *ShutdownTestSuite) TestSecondInterruptDuringAMethod(t *gotest.T) {
	markers := t.TempDir()
	run := s.start(t, markers, "held")
	interruptWhen(t, run, markers, "test-running")
	first := time.Now()
	// Past the window of a repeat, well inside the method's 5s grace.
	time.Sleep(time.Until(first.Add(time.Second)))
	gotest.NoError(t, testkit.Interrupt(run.cmd, run.tree))

	exited := make(chan int, 1)
	go func() { exited <- exitCode(t, run) }()
	var code int
	select {
	case code = <-exited:
	case <-time.After(10 * time.Second):
		_ = run.tree.Kill()
		gotest.Fail(t, "the CLI was still running 10s after the second interrupt")
	}

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
	t.It("leaves no shared fixture process behind", func(it *gotest.T) {
		data, err := os.ReadFile(filepath.Join(markers, "fixture-tearing-down"))
		gotest.NoError(it, err)
		setup, err := strconv.Atoi(string(data))
		gotest.NoError(it, err)
		gotest.True(it, testkit.EndedWithin(setup, 2*time.Second), "the shared fixture process %d outlived the CLI", setup)
	})
}

// The spec, summary and bench commands run the batch pipeline, not the
// streaming one; an interrupt must mean 130 on both.
func (s *ShutdownTestSuite) TestInterruptDuringSpec(t *gotest.T) {
	markers := t.TempDir()
	run := s.startWith(t, markers, nil, "spec", "./slow/")
	interruptWhen(t, run, markers, "test-running")
	code := exitCode(t, run)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

// Once every verdict is in, an interrupt has nothing left to cut short: a
// green run that is interrupted while its fixture tears down is still green.
func (s *ShutdownTestSuite) TestInterruptDuringTeardown(t *gotest.T) {
	markers := t.TempDir()
	run := s.startWith(t, markers, []string{"GOTEST_SHUTDOWN_TEARDOWN_SLEEP=3s"}, "./quick/")
	interruptWhen(t, run, markers, "fixture-tearing-down")
	code := exitCode(t, run)

	t.It("keeps the suites' verdict", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
	})
	t.It("lets the teardown finish", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not finish")
	})
}

// globalBudget is the first --timeout TestGlobalTimeout gives a run; a run
// it ends before TestSleeps starts is retried with double, budgetTries in all.
const (
	globalBudget = 15 * time.Second
	budgetTries  = 3
)

// timeoutCase is one pipeline TestGlobalTimeout cuts short.
type timeoutCase struct {
	Desc    string
	args    []string
	running string
	shows   string
	budget  time.Duration
	early   []time.Duration // budgets that ran out before TestSleeps started
	markers string
	out     *bytes.Buffer
	run     *cliRun
	code    int
}

// A --timeout that expires mid-run fails it with 1 on the streaming and the
// batch pipeline alike and names what was still running: the method where a
// stream exists, the suite in the text run. It is not censused.
// The claim needs the deadline to fire while TestSleeps sleeps: a run whose
// budget ran out on linking or the fixture's setup has no test-running marker
// and says nothing about it, so it is run again with double the budget. The
// method sleeps 60s, past the last budget. The runs of each round overlap.
func (s *ShutdownTestSuite) TestGlobalTimeout(t *gotest.T) {
	cases := []timeoutCase{
		{Desc: "gotest ./...", args: []string{"./slow/"}, running: "shutdownmod/slow TestSlowTestSuite\n"},
		{
			Desc: "gotest -json", args: []string{"-json", "./slow/"}, running: "shutdownmod/slow TestSlowTestSuite/TestSleeps\n",
			shows: `"Action":"fail","Package":"shutdownmod/slow","Test":"TestSlowTestSuite/TestSleeps"`,
		},
		{
			Desc: "gotest spec", args: []string{"spec", "--no-color", "./slow/"}, running: "shutdownmod/slow TestSlowTestSuite/TestSleeps\n",
			shows: "✗ Sleeps",
		},
	}
	pending := make([]*timeoutCase, len(cases))
	for i := range cases {
		pending[i] = &cases[i]
	}
	for try := 1; len(pending) > 0; try++ {
		if try > budgetTries {
			c := pending[0]
			gotest.Fail(t, "%s: the --timeout fired before TestSleeps started in all %d tries, the last at %v, so it had nothing running to name:\n%s",
				c.Desc, budgetTries, c.budget/2, c.out)
		}
		for _, c := range pending {
			if c.budget == 0 {
				c.budget = globalBudget
			}
			c.markers = t.TempDir()
			// The flag goes before the package: a subcommand comes first.
			pkg := len(c.args) - 1
			args := append(append(slices.Clone(c.args[:pkg]), "--timeout="+c.budget.String()), c.args[pkg])
			cmd := s.command(c.markers, nil, args)
			c.out = new(bytes.Buffer)
			cmd.Stdout, cmd.Stderr = c.out, c.out
			c.run = s.launch(t, cmd)
		}
		var inconclusive []*timeoutCase
		for _, c := range pending {
			c.code = exitCode(t, c.run)
			if !marker(c.markers, "test-running") {
				c.early = append(c.early, c.budget)
				c.budget *= 2
				inconclusive = append(inconclusive, c)
			}
		}
		pending = inconclusive
	}
	for sub, tc := range gotest.Each(t, cases) { //nolint:gocritic // rangeValCopy: intentional
		out := tc.out.String()
		gotest.Equal(sub, 1, tc.code, out)
		gotest.Contains(sub, out, "FAIL: global --timeout exceeded after "+tc.budget.String()+" while running: "+tc.running,
			"budgets that ran out before TestSleeps started: %v", tc.early)
		gotest.Contains(sub, out, tc.shows)
		gotest.NotContains(sub, out, "=== global --timeout")
		gotest.NotContains(sub, out, "census")
		gotest.True(sub, marker(tc.markers, "fixture-down"), "AfterAll did not run")
	}
}
