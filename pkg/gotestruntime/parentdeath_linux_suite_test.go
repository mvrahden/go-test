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

// A run of this test binary with envOrphanMode set is one process of the
// chain a runner builds: "runner" starts a "converter" through a tree, the
// way the CLI starts test2json, and the converter starts the suite process.
// The converter neither handles nor passes on a signal, as test2json does.
const (
	envOrphanMode   = "GOTESTRUNTIME_ORPHAN_MODE"
	envOrphanDir    = "GOTESTRUNTIME_ORPHAN_DIR"
	orphanSuite     = "TestParentDeathTestSuite"
	orphanMethod    = "TestASuiteWhoseRunnerIsKilledTearsDown"
	orphanPIDFile   = "suite.pid"
	orphanTornFile  = "torn-down"
	orphanStopFile  = "suite.stop"
	orphanChildMode = "suite"
)

func init() {
	switch os.Getenv(envOrphanMode) {
	case "runner":
		os.Exit(orphanRunner())
	case "converter":
		os.Exit(orphanConverter())
	}
}

func orphanRunner() int {
	cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), envOrphanMode+"=converter")
	if proctree.New(cmd).Start() != nil {
		return 2
	}
	time.Sleep(5 * time.Minute)
	return 0
}

func orphanConverter() int {
	dir := os.Getenv(envOrphanDir)
	cmd := exec.Command(os.Args[0], //nolint:gosec // G204: this test binary
		"-test.run=^"+orphanSuite+"$/^"+orphanMethod+"$", "-test.v", "-test.count=1")
	cmd.Env = []string{envOrphanMode + "=" + orphanChildMode, protocol.EnvStopFile + "=" + filepath.Join(dir, orphanStopFile)}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envOrphanMode+"=") {
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

// orphanSuiteRun is the suite process: a suite whose method waits for its
// context to end and whose AfterAll leaves a marker.
func orphanSuiteRun(t *testing.T) {
	dir := os.Getenv(envOrphanDir)
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, orphanPIDFile), []byte(strconv.Itoa(os.Getpid())), 0o600)
			},
			AfterAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, orphanTornFile), []byte("torn down"), 0o600)
			},
		})
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
			Name: "TestWaits",
			Run: func(ttt *gotest.T, run func(func())) {
				run(func() { <-ttt.Context().Done() })
			},
		}}})
	})
}

// ParentDeathTestSuite: a suite process whose runner is killed tears down,
// though the converter in front of it dies without passing anything on.
// Sequential: the scenario starts and kills processes.
type ParentDeathTestSuite struct{}

func (s *ParentDeathTestSuite) TestASuiteWhoseRunnerIsKilledTearsDown(t *gotest.T) {
	if os.Getenv(envOrphanMode) == orphanChildMode {
		orphanSuiteRun(t.T())
		return
	}
	dir := t.TempDir()
	runner := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	runner.Env = append(os.Environ(), envOrphanMode+"=runner", envOrphanDir+"="+dir)
	gotest.NoError(t, runner.Start())
	pid := 0
	gotest.Eventually(t, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
		b, err := os.ReadFile(filepath.Join(dir, orphanPIDFile))
		gotest.NoError(poll, err, "the suite process never opened its suite")
		pid, err = strconv.Atoi(string(b))
		gotest.NoError(poll, err)
	})
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }() // never leave it behind when the check fails
	gotest.NoError(t, runner.Process.Kill())
	_ = runner.Wait()

	t.When("the runner is killed while a suite method runs behind a converter", func(w *gotest.T) {
		w.It("the suite's AfterAll runs, then its process is gone", func(it *gotest.T) {
			gotest.Eventually(it, 20*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, processGone(pid), "the suite process outlived its runner")
			})
			_, err := os.Stat(filepath.Join(dir, orphanTornFile))
			gotest.NoError(it, err, "the suite process was gone without its AfterAll")
		})
	})
}

// processGone reports whether pid has exited: no such process, or one only
// waiting to be reaped.
func processGone(pid int) bool {
	if syscall.Kill(pid, 0) == syscall.ESRCH {
		return true
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	end := strings.LastIndexByte(string(stat), ')')
	return end > 0 && end+2 < len(stat) && stat[end+2] == 'Z'
}
