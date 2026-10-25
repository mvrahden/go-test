//go:build linux || darwin

package gotestruntime_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envPipeMode set is one process of the
// chain below: a "converter" reads the suite's stream and starts a shell
// wrapper that runs the suite process and waits for it. The converter is
// killed; the wrapper, the suite's parent, lives on.
const (
	envPipeMode   = "GOTESTRUNTIME_PIPE_MODE"
	envPipeDir    = "GOTESTRUNTIME_PIPE_DIR"
	pipeSuite     = "TestReaderGoneTestSuite"
	pipeChildMode = "suite"
)

func init() {
	if os.Getenv(envPipeMode) == "converter" {
		os.Exit(pipeConverter())
	}
}

func pipeConverter() int {
	dir := os.Getenv(envPipeDir)
	// The trailing exit keeps the shell from replacing itself with the suite.
	cmd := exec.Command("sh", "-c", `"$@"; exit $?`, "sh", os.Args[0], //nolint:gosec // G204: this test binary
		"-test.run=^"+pipeSuite+"$/^TestASuiteWhoseReaderIsKilledTearsDown$", "-test.v", "-test.count=1")
	cmd.Env = []string{envPipeMode + "=" + pipeChildMode, protocol.EnvStopFile + "=" + filepath.Join(dir, "suite.stop")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envPipeMode+"=") {
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

// pipeSuiteRun is the suite process: a method that waits for its context to
// end and says so, and an AfterAll that leaves a marker.
func pipeSuiteRun(t *testing.T) {
	dir := os.Getenv(envPipeDir)
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, "suite.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
			},
			AfterAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, "torn-down"), nil, 0o600)
			},
		})
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{{
			Name: "TestWaits",
			Run: func(ttt *gotest.T, run func(func())) {
				run(func() {
					<-ttt.Context().Done()
					_ = os.WriteFile(filepath.Join(dir, "stopped"), nil, 0o600)
				})
			},
		}}})
	})
}

// ReaderGoneTestSuite: a suite process under the runner whose stream nobody
// reads any more stops and tears down, though its parent lives on.
// Sequential: the scenario starts and kills processes.
type ReaderGoneTestSuite struct{}

// SuiteConfig: a method took at most 0.3s locally; seven times that, twice
// over, is well under the default.
func (s *ReaderGoneTestSuite) SuiteConfig() gotest.SuiteConfig { return gotest.DefaultSuiteConfig() }

func (s *ReaderGoneTestSuite) TestASuiteWhoseReaderIsKilledTearsDown(t *gotest.T) {
	if os.Getenv(envPipeMode) == pipeChildMode {
		pipeSuiteRun(t.T())
		return
	}
	dir := t.TempDir()
	converter := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	converter.Env = append(os.Environ(), envPipeMode+"=converter", envPipeDir+"="+dir)
	gotest.NoError(t, converter.Start())
	pid := 0
	gotest.Eventually(t, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
		b, err := os.ReadFile(filepath.Join(dir, "suite.pid"))
		gotest.NoError(poll, err, "the suite process never opened its suite")
		pid, err = strconv.Atoi(string(b))
		gotest.NoError(poll, err)
	})
	defer func() { // never leave it behind when the check fails
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}()
	gotest.NoError(t, converter.Process.Kill())
	_ = converter.Wait()

	t.When("the reader of a suite's stream is killed while a wrapper waits on the suite", func(w *gotest.T) {
		w.It("the method's context ends well before its timeout, then AfterAll runs", func(it *gotest.T) {
			gotest.Eventually(it, 15*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				_, err := os.Stat(filepath.Join(dir, "torn-down"))
				gotest.NoError(poll, err, "the suite process did not tear down after its reader ended")
			})
			_, err := os.Stat(filepath.Join(dir, "stopped"))
			gotest.NoError(it, err)
		})
	})
}
