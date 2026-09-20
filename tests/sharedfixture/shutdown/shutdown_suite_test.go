package shutdown_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	cfg.Timeout = 3 * time.Minute
	cfg.SetupTimeout = 2 * time.Minute
	return cfg
}

func (s *ShutdownTestSuite) BeforeAll(t *gotest.T) {
	testkit.ScrubActionsEnv()
	s.binary = s.CLI.Binary
	s.module = t.TempDir()
	src := filepath.Join(s.CLI.RepoRoot, "tests", "sharedfixture", "testdata", "shutdownmod")
	gotest.NoError(t, testkit.StageModule(s.CLI.RepoRoot, src, s.module))
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

// runWith runs the CLI to completion and returns its combined output and
// exit code.
func (s *ShutdownTestSuite) runWith(t *gotest.T, markers string, args ...string) (string, int) {
	cmd := s.command(markers, nil, args)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	code := exitCode(t, s.launch(t, cmd))
	return out.String(), code
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

// A --timeout that expires mid-run fails it with 1 on the streaming and the
// batch pipeline alike and names what was still running: the method where a
// stream exists, the suite in the text run. It is not censused.
func (s *ShutdownTestSuite) TestGlobalTimeout(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct { //nolint:gocritic // rangeValCopy: intentional
		Desc    string
		args    []string
		running string
		shows   string
	}{
		{Desc: "gotest ./...", args: []string{"--timeout=20s", "./slow/"}, running: "shutdownmod/slow TestSlowTestSuite\n"},
		{
			Desc: "gotest -json", args: []string{"-json", "--timeout=20s", "./slow/"}, running: "shutdownmod/slow TestSlowTestSuite/TestSleeps\n",
			shows: `{"Action":"fail","Package":"shutdownmod/slow","Test":"TestSlowTestSuite/TestSleeps"}`,
		},
		{
			Desc: "gotest spec", args: []string{"spec", "--no-color", "--timeout=20s", "./slow/"}, running: "shutdownmod/slow TestSlowTestSuite/TestSleeps\n",
			shows: "✗ Sleeps",
		},
	}) {
		markers := sub.TempDir()
		out, code := s.runWith(sub, markers, tc.args...)
		gotest.Equal(sub, 1, code, out)
		gotest.Contains(sub, out, "FAIL: global --timeout exceeded after 20s while running: "+tc.running)
		gotest.Contains(sub, out, tc.shows)
		gotest.NotContains(sub, out, "=== global --timeout")
		gotest.NotContains(sub, out, "census")
		gotest.True(sub, marker(markers, "fixture-down"), "AfterAll did not run")
	}
}
