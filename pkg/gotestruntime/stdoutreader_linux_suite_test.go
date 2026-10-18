//go:build linux

package gotestruntime_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envReaderMode set is one process of the
// chain a runner builds under -json with an -exec wrapper: "runner" starts a
// "converter" through a tree, the converter reads the stream and passes no
// signal on, and starts a shell wrapper that runs the suite process and
// waits for it. The wrapper outlives the converter, so the suite's parent
// never goes away.
const (
	envReaderMode   = "GOTESTRUNTIME_READER_MODE"
	envReaderDir    = "GOTESTRUNTIME_READER_DIR"
	envReaderMethod = "GOTESTRUNTIME_READER_METHOD"
	envReaderWait   = "GOTESTRUNTIME_READER_WAIT"
	envReaderBare   = "GOTESTRUNTIME_READER_BARE" // no stop file: plain go test
	readerSuite     = "TestStdoutReaderTestSuite"
	readerChild     = "suite"
	readerPIDs      = "suite.pids"
	readerConverter = "converter.pid"
	readerTorn      = "torn-down"
	readerOutcome   = "outcome"
)

func init() {
	switch os.Getenv(envReaderMode) {
	case "runner":
		os.Exit(readerRunner())
	case "converter":
		os.Exit(readerConverterRun())
	}
}

func readerRunner() int {
	cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), envReaderMode+"=converter")
	if proctree.New(cmd).Start() != nil {
		return 2
	}
	time.Sleep(5 * time.Minute)
	return 0
}

func readerConverterRun() int {
	dir := os.Getenv(envReaderDir)
	_ = os.WriteFile(filepath.Join(dir, readerConverter), []byte(strconv.Itoa(os.Getpid())), 0o600)
	// The trailing exit keeps the shell from replacing itself with the suite.
	cmd := exec.Command("sh", "-c", `"$@"; exit $?`, "sh", os.Args[0], //nolint:gosec // G204: this test binary
		"-test.run=^"+readerSuite+"$/^"+os.Getenv(envReaderMethod)+"$", "-test.v", "-test.count=1")
	cmd.Env = []string{envReaderMode + "=" + readerChild}
	if os.Getenv(envReaderBare) == "" {
		cmd.Env = append(cmd.Env, protocol.EnvStopFile+"="+filepath.Join(dir, "suite.stop"))
	}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envReaderMode+"=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return 2
	}
	_, _ = io.Copy(io.Discard, out)
	_ = cmd.Wait()
	return 0
}

// readerSuiteRun is the suite process: a method that waits for its context
// to end or for envReaderWait to pass, and says which came first.
func readerSuiteRun(t *testing.T) {
	dir := os.Getenv(envReaderDir)
	wait, _ := time.ParseDuration(os.Getenv(envReaderWait))
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {
				pids := strconv.Itoa(os.Getpid()) + " " + strconv.Itoa(os.Getppid())
				_ = os.WriteFile(filepath.Join(dir, readerPIDs), []byte(pids), 0o600)
			},
			AfterAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, readerTorn), []byte("torn down"), 0o600)
			},
		})
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
			Name: "TestWaits",
			Run: func(ttt *gotest.T, run func(func())) {
				run(func() {
					outcome := "finished"
					select {
					case <-ttt.Context().Done():
						outcome = "stopped"
					case <-time.After(wait):
					}
					_ = os.WriteFile(filepath.Join(dir, readerOutcome), []byte(outcome), 0o600)
				})
			},
		}}})
	})
}

// StdoutReaderTestSuite: a suite process whose stream nobody reads any more
// stops and tears down, though its parent, an -exec wrapper, is still there.
// Sequential: the scenarios start and kill processes.
type StdoutReaderTestSuite struct{}

// readerChain starts the chain for method and returns the runner, the
// directory the processes report into and the suite's pid and parent pid.
func readerChain(t *gotest.T, method string, wait time.Duration, bare bool) (runner *exec.Cmd, dir string, pid, ppid int) {
	dir = t.TempDir()
	runner = exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	runner.Env = append(os.Environ(), envReaderMode+"=runner", envReaderDir+"="+dir,
		envReaderMethod+"="+method, envReaderWait+"="+wait.String())
	if bare {
		runner.Env = append(runner.Env, envReaderBare+"=1")
	}
	gotest.NoError(t, runner.Start())
	gotest.Eventually(t, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
		b, err := os.ReadFile(filepath.Join(dir, readerPIDs))
		gotest.NoError(poll, err, "the suite process never opened its suite")
		fields := strings.Fields(string(b))
		gotest.Len(poll, fields, 2)
		pid, err = strconv.Atoi(fields[0])
		gotest.NoError(poll, err)
		ppid, err = strconv.Atoi(fields[1])
		gotest.NoError(poll, err)
	})
	b, err := os.ReadFile(filepath.Join(dir, readerConverter))
	gotest.NoError(t, err)
	gotest.NotEqual(t, strings.TrimSpace(string(b)), strconv.Itoa(ppid), "the wrapper is not between the converter and the suite")
	return runner, dir, pid, ppid
}

func killRunner(runner *exec.Cmd) {
	_ = runner.Process.Kill()
	_ = runner.Wait()
}

func readOutcome(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, readerOutcome))
	return string(b)
}

func (s *StdoutReaderTestSuite) TestAWrappedSuiteWhoseReaderIsGoneTearsDown(t *gotest.T) {
	if os.Getenv(envReaderMode) == readerChild {
		readerSuiteRun(t.T())
		return
	}
	runner, dir, pid, _ := readerChain(t, "TestAWrappedSuiteWhoseReaderIsGoneTearsDown", 5*time.Minute, false)
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }() // never leave it behind when the check fails
	killRunner(runner)

	t.When("the runner and the converter die while an -exec wrapper waits on the suite", func(w *gotest.T) {
		w.It("the suite's method is stopped, its AfterAll runs, then its process is gone", func(it *gotest.T) {
			gotest.Eventually(it, 20*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(pid), "the suite process outlived the reader of its stream")
			})
			gotest.Equal(it, "stopped", readOutcome(dir))
			_, err := os.Stat(filepath.Join(dir, readerTorn))
			gotest.NoError(it, err, "the suite process was gone without its AfterAll")
		})
	})
}

func (s *StdoutReaderTestSuite) TestAWrappedSuiteWhoseReaderLivesRunsOn(t *gotest.T) {
	if os.Getenv(envReaderMode) == readerChild {
		readerSuiteRun(t.T())
		return
	}
	runner, dir, pid, _ := readerChain(t, "TestAWrappedSuiteWhoseReaderLivesRunsOn", 2*time.Second, false)
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	defer killRunner(runner)

	t.When("the converter keeps reading the suite's stream", func(w *gotest.T) {
		w.It("the suite's method runs to its end", func(it *gotest.T) {
			gotest.Eventually(it, 20*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(pid), "the suite process never finished")
			})
			gotest.Equal(it, "finished", readOutcome(dir))
		})
	})
}

func (s *StdoutReaderTestSuite) TestASuiteOutsideTheRunnerIgnoresAGoneReader(t *gotest.T) {
	if os.Getenv(envReaderMode) == readerChild {
		readerSuiteRun(t.T())
		return
	}
	runner, dir, pid, _ := readerChain(t, "TestASuiteOutsideTheRunnerIgnoresAGoneReader", 2*time.Second, true)
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	killRunner(runner)

	t.When("the reader of a plain go test run's stream is gone", func(w *gotest.T) {
		w.It("the suite's method is not stopped", func(it *gotest.T) {
			gotest.Eventually(it, 20*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(pid), "the suite process never finished")
			})
			gotest.Equal(it, "finished", readOutcome(dir))
		})
	})
}
