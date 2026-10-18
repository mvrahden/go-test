package gotestruntime_test

import (
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

// A run of this test binary with envGoneMode set is one process of the
// scenario below: "parent" starts the suite process and waits for it, and is
// killed meanwhile. The suite's stream goes to a file, so only the parent's
// end can stop it.
const (
	envGoneMode   = "GOTESTRUNTIME_GONE_MODE"
	envGoneDir    = "GOTESTRUNTIME_GONE_DIR"
	envGoneSlow   = "GOTESTRUNTIME_GONE_SLOW"
	goneSuite     = "TestParentGoneTestSuite"
	goneMethod    = "TestASuiteWhoseParentIsKilledTearsDown"
	goneChildMode = "suite"
)

func init() {
	if os.Getenv(envGoneMode) == "parent" {
		os.Exit(goneParent())
	}
}

func goneParent() int {
	dir := os.Getenv(envGoneDir)
	out, err := os.Create(filepath.Join(dir, "suite.out"))
	if err != nil {
		return 2
	}
	cmd := exec.Command(os.Args[0], //nolint:gosec // G204: this test binary
		"-test.run=^"+goneSuite+"$/^"+goneMethod+"$", "-test.v", "-test.count=1")
	cmd.Env = []string{envGoneMode + "=" + goneChildMode, protocol.EnvStopFile + "=" + filepath.Join(dir, "suite.stop")}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envGoneMode+"=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Stdout, cmd.Stderr = out, out
	if cmd.Start() != nil {
		return 2
	}
	_ = cmd.Wait()
	return 0
}

// goneSuiteRun is the suite process: a method that waits for its context to
// end and says so, and an AfterAll that leaves a marker.
func goneSuiteRun(t *testing.T) {
	dir := os.Getenv(envGoneDir)
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, "suite.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
			},
			AfterAll: func(*gotest.T) {
				if os.Getenv(envGoneSlow) == "1" {
					time.Sleep(2 * time.Second) // long enough for the parent to die meanwhile
				}
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

// ParentGoneTestSuite: a suite process under the runner whose parent is
// killed stops and tears down, though no signal and no closed stream says so.
// Sequential: the scenario starts and kills processes.
type ParentGoneTestSuite struct{}

func (s *ParentGoneTestSuite) TestASuiteWhoseParentIsKilledTearsDown(t *gotest.T) {
	if os.Getenv(envGoneMode) == goneChildMode {
		goneSuiteRun(t.T())
		return
	}
	dir := t.TempDir()
	parent := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	parent.Env = append(os.Environ(), envGoneMode+"=parent", envGoneDir+"="+dir)
	gotest.NoError(t, parent.Start())
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
	gotest.NoError(t, parent.Process.Kill())
	_ = parent.Wait()

	t.When("the parent of a suite process under the runner is killed while a method runs", func(w *gotest.T) {
		w.It("the method's context ends well before its timeout, then AfterAll runs", func(it *gotest.T) {
			gotest.Eventually(it, 15*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				_, err := os.Stat(filepath.Join(dir, "torn-down"))
				gotest.NoError(poll, err, "the suite process did not tear down after its parent ended")
			})
			_, err := os.Stat(filepath.Join(dir, "stopped"))
			gotest.NoError(it, err)
		})
	})
}
