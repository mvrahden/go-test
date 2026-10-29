//go:build unix

package fixtures_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// KilledCLITestSuite: a shared fixture process whose CLI is killed releases
// what it holds and does not stay behind.
type KilledCLITestSuite struct {
	cli string
}

// SuiteConfig: locally a method took 1.4s, under the default seven times
// slower; BeforeAll builds the CLI, 8.5s cold, so twice that seven times
// slower is 2m.
func (s *KilledCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 45 * time.Second}
}

func (s *KilledCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = buildKillableCLI(t)
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *KilledCLITestSuite) AfterAll(t *gotest.T) {}

func buildKillableCLI(t *gotest.T) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	cli, err := testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	return cli
}

// killedAt runs the fixbkilled package under cli with env, waits for the
// marker ready, kills the CLI alone with SIGKILL and returns the shared
// fixture process's pid and when the CLI was killed. A run that never gets
// there is killed before the test fails.
func killedAt(t *gotest.T, cli, markerDir, ready string, env ...string) (int, time.Time) {
	cmd := exec.Command(cli, "-json", "--setup-timeout=10m", "../testdata/fixbkilled/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir, env...)
	out := new(strings.Builder)
	cmd.Stdout, cmd.Stderr = out, out
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, ready) {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never reached %q:\n%s", ready, out) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
	gotest.NoError(t, cmd.Process.Kill())
	killed := time.Now()
	_ = cmd.Wait()
	tree.Release()
	got, _ := testkit.ReadMarkers(markers)
	for _, m := range got {
		if v, ok := strings.CutPrefix(m, "pid="); ok {
			pid, err := strconv.Atoi(v)
			gotest.NoError(t, err)
			return pid, killed
		}
	}
	gotest.Fail(t, "the fixture marked no pid: %v", got)
	return 0, killed
}

// gone reports whether no process has pid any more.
func gone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

func (s *KilledCLITestSuite) TestKilledWhileUp(t *gotest.T) {
	markerDir := t.TempDir()
	pid, _ := killedAt(t, s.cli, markerDir, "suite holds")
	t.When("the CLI is killed while a shared fixture is up", func(w *gotest.T) {
		w.It("the fixture's AfterAll runs to its end, then its process exits", func(it *gotest.T) {
			gotest.Eventually(it, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, gone(pid), "the shared fixture process outlived the CLI")
			})
			markers, _ := testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
			gotest.Contains(it, markers, "AfterAll ends")
		})
	})
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func (s *KilledCLITestSuite) TestKilledDuringSetup(t *gotest.T) {
	markerDir := t.TempDir()
	pid, _ := killedAt(t, s.cli, markerDir, "setup waiting", "FIXB_SETUP=wait")
	t.When("the CLI is killed while a shared fixture's BeforeAll waits on its context", func(w *gotest.T) {
		w.It("the setup's context ends, the fixture's AfterAll runs, then its process exits", func(it *gotest.T) {
			gotest.Eventually(it, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
				gotest.True(poll, gone(pid), "the shared fixture process outlived the CLI")
			})
			markers, _ := testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
			gotest.True(it, testkit.MarkerOrderOf(markers, "setup context ended", "AfterAll ends"), "markers: %v", markers)
		})
	})
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
