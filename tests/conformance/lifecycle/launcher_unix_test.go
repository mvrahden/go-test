//go:build unix

package lifecycle_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LauncherSignalTestSuite: however gotest was started and whichever signal
// stops it, the fixtures are released, a second signal abandons what is left
// of the teardown, and no process outlives the run. Every row tears down
// slowly, so a teardown nobody waited for shows as one.
// Sequential: it stages a module once and every row drives a live process.
type LauncherSignalTestSuite struct {
	cli    string
	staged string
}

func (s *LauncherSignalTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Timeout = 5 * time.Minute
	return cfg
}

func (s *LauncherSignalTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	// A module of its own, requiring gotest from this checkout and declaring
	// the tool, with the stopping package copied in.
	s.staged = t.TempDir()
	gotest.NoError(t, testkit.WriteModule(root, s.staged, "example.com/launch"))
	goMod := filepath.Join(s.staged, "go.mod")
	data, err := os.ReadFile(goMod)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(goMod, append(data, []byte("\ntool github.com/mvrahden/go-test/cmd/gotest\n")...), 0o600))
	gotest.NoError(t, os.MkdirAll(filepath.Join(s.staged, "stopping"), 0o750))
	for _, name := range []string{"fixture.go", "stopping_suite_test.go"} {
		src, err := os.ReadFile(filepath.Join("..", "testdata", "stopping", name))
		gotest.NoError(t, err)
		gotest.NoError(t, os.WriteFile(filepath.Join(s.staged, "stopping", name), src, 0o600))
	}
}

// AfterAll has nothing to release: the binary and the module live in temp dirs.
func (s *LauncherSignalTestSuite) AfterAll(t *gotest.T) {}

type launcherRow struct {
	Desc     string
	launcher string // direct, go run, go tool
	signal   string // SIGINT, SIGTERM, second
}

// command builds the row's launcher invocation over the staged package.
func (s *LauncherSignalTestSuite) command(launcher string) *exec.Cmd {
	var cmd *exec.Cmd
	switch launcher {
	case "go run":
		cmd = exec.Command("go", "run", "github.com/mvrahden/go-test/cmd/gotest", "-json", "./stopping/")
	case "go tool":
		cmd = exec.Command("go", "tool", "gotest", "-json", "./stopping/")
	default:
		cmd = exec.Command(s.cli, "-json", "./stopping/") //nolint:gosec // G204: binary built by this suite
	}
	cmd.Dir = s.staged
	return cmd
}

// deliver sends the row's signal to the run's process group, as a terminal
// (SIGINT) or a job supervisor (SIGTERM) would; "second" is Ctrl-C twice,
// the second landing while the teardown runs.
func deliver(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, signal string) {
	group := -cmd.Process.Pid
	switch signal {
	case "SIGTERM":
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGTERM))
	case "second":
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGINT))
		time.Sleep(500 * time.Millisecond)
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGINT))
	default:
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGINT))
	}
}

// pidOf reads the suite process's pid from the markers.
func pidOf(markers []string) int {
	for _, m := range markers {
		if n, ok := strings.CutPrefix(m, "pid="); ok {
			pid, _ := strconv.Atoi(n)
			return pid
		}
	}
	return 0
}

// gone reports whether the suite process's group, which it leads, is no
// process within a short grace period. A group id is not reused while a
// member is left, so the probe reaches no stranger.
func gone(pid int) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := syscall.Kill(-pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
	}
	return false
}

func (s *LauncherSignalTestSuite) TestEveryLauncherStopsCleanly(t *gotest.T) {
	// Each cell names the launcher and the signal; the row pins that the
	// fixture's teardown finished (or, on a second signal, was abandoned with
	// the CLI saying so), the exit code, and that the suite process is gone
	// once the launcher has returned. Exit codes: the direct binary and go
	// tool end with 130; go run swallows SIGINT, waits, and reports "exit
	// status 130" with exit 1, while a group SIGTERM ends go run itself by
	// the signal.
	t.When("a run is stopped by a signal, however gotest was launched", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []launcherRow{
			{Desc: "direct, SIGINT", launcher: "direct", signal: "SIGINT"},
			{Desc: "direct, SIGTERM", launcher: "direct", signal: "SIGTERM"},
			{Desc: "direct, a second SIGINT", launcher: "direct", signal: "second"},
			{Desc: "go run, SIGINT", launcher: "go run", signal: "SIGINT"},
			{Desc: "go run, SIGTERM", launcher: "go run", signal: "SIGTERM"},
			{Desc: "go run, a second SIGINT", launcher: "go run", signal: "second"},
			{Desc: "go tool, SIGINT", launcher: "go tool", signal: "SIGINT"},
			{Desc: "go tool, SIGTERM", launcher: "go tool", signal: "SIGTERM"},
			{Desc: "go tool, a second SIGINT", launcher: "go tool", signal: "second"},
		}) {
			markerDir := it.TempDir()
			cmd := s.command(row.launcher)
			cmd.Env = testkit.ChildEnv(markerDir, "STOPPING_SLOW=1")
			tree := holding(it, cmd, markerDir)
			deliver(it, cmd, tree, row.signal)
			run := finished(cmd, tree, markerDir)

			switch {
			case row.launcher == "go run" && row.signal == "SIGTERM":
				gotest.Equal(it, -1, run.Exit, "go run ends by the signal; stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			case row.launcher == "go run":
				gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(it, run.Stderr, "exit status 130")
			default:
				gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			}
			if row.signal == "second" {
				gotest.Contains(it, run.Stderr, "interrupted again")
				gotest.NotContains(it, run.Markers, "teardown done")
			} else {
				gotest.NotContains(it, run.Stderr, "interrupted again")
				gotest.Contains(it, run.Markers, "teardown done")
			}
			pid := pidOf(run.Markers)
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		}
	})
}
