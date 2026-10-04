//go:build !windows

package canary_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InterruptTestSuite stops a run while a fixture-bound test holds it open.
// Go's default for SIGINT and SIGTERM ends a process on the spot, so the
// fixture is released only if the test binary takes the signal itself.
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
	s.binary = filepath.Join(t.TempDir(), "interrupt.test")
	out, err = exec.Command("go", "test", "-c", "-o", s.binary, "./testdata/interrupt/").CombinedOutput() //nolint:gosec // G204: fixed args
	gotest.NoError(t, err, string(out))
}

func (s *InterruptTestSuite) AfterAll(t *gotest.T) {
	generated, _ := filepath.Glob("testdata/interrupt/gotest_p*suite_test.go")
	for _, f := range generated {
		_ = os.Remove(f)
	}
}

// started starts cmd logging to a fresh file and returns once the test holds
// the fixture.
func started(t *gotest.T, cmd *exec.Cmd) (log string) {
	log = filepath.Join(t.TempDir(), "events")
	cmd.Env = append(cmd.Env, "INTERRUPT_LOG="+log)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	// Its own group, as a terminal gives a foreground job.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	gotest.NoError(t, cmd.Start())
	gotest.Eventually(t, 30*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
		gotest.Contains(poll, readEvents(log), "running")
	})
	return log
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

func readEvents(log string) []string {
	b, _ := os.ReadFile(log)
	return strings.Fields(string(b))
}

// endedBy reports whether a waited-for process ended by sig, or, when it
// started with sig ignored and could not, by the status a shell gives for it.
func endedBy(err error, sig syscall.Signal) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return ws.Signal() == sig
	}
	return exit.ExitCode() == 128+int(sig)
}

func (s *InterruptTestSuite) TestCtrlCReleasesTheFixture(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = ownEnv()
	log := started(t, cmd)

	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err := cmd.Wait()

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.True(t, endedBy(err, syscall.SIGINT), "the process still ends by the signal: %v", err)
}

// Under the runner a signal is a stop request only once the runner announced
// it; the runner creates the stop file before it interrupts.
func (s *InterruptTestSuite) TestAnAnnouncedStopReleasesTheFixture(t *gotest.T) {
	stop := filepath.Join(t.TempDir(), "stop")
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), protocol.EnvStopFile+"="+stop)
	log := started(t, cmd)

	gotest.NoError(t, os.WriteFile(stop, nil, 0o600))
	gotest.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	err := cmd.Wait()

	gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log))
	gotest.True(t, endedBy(err, syscall.SIGTERM), "the process still ends by the signal: %v", err)
}

// A signal the runner did not announce came from somewhere else, such as code
// under test signalling its own process; it is left to that code.
func (s *InterruptTestSuite) TestAnUnannouncedSignalIsLeftAlone(t *gotest.T) {
	cmd := exec.Command(s.binary) //nolint:gosec // G204: binary built by this suite
	cmd.Env = append(ownEnv(), protocol.EnvStopFile+"="+filepath.Join(t.TempDir(), "stop"))
	log := started(t, cmd)

	gotest.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	gotest.Consistently(t, 500*time.Millisecond, 50*time.Millisecond, func(poll *gotest.R) {
		gotest.Empty(poll, exited, "the process ended on a signal nobody announced")
	})

	gotest.NoError(t, cmd.Process.Kill())
	<-exited
	gotest.Equal(t, []string{"setup", "running"}, readEvents(log))
}

// The runner announces the stop and waits for the suite process, including
// past test2json, which ends on the signal before the binary it runs.
func (s *InterruptTestSuite) TestTheRunnerWaitsForTheRelease(t *gotest.T) {
	for _, flags := range [][]string{nil, {"-json"}} {
		cmd := exec.Command(s.cli, append(flags, "./testdata/interrupt/")...) //nolint:gosec // G204: controlled binary with fixed args
		cmd.Env = ownEnv()
		log := started(t, cmd)

		gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
		_ = cmd.Wait()

		gotest.Equal(t, 130, cmd.ProcessState.ExitCode(), "gotest %v", flags)
		gotest.Equal(t, []string{"setup", "running", "teardown"}, readEvents(log), "gotest %v: released before the CLI exited", flags)
	}
}
