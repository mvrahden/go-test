//go:build unix

package lifecycle_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LauncherSignalTestSuite: however gotest was started and whichever signal
// stops it, the fixtures are released and no process outlives the run.
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
// or a job supervisor would.
func deliver(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, signal string) {
	switch signal {
	case "SIGTERM":
		gotest.NoError(t, syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM))
	case "second":
		gotest.NoError(t, tree.Interrupt())
		time.Sleep(500 * time.Millisecond)
		gotest.NoError(t, tree.Interrupt())
	default:
		gotest.NoError(t, tree.Interrupt())
	}
}

// pidOf reads the suite process's pid from the markers.
func pidOf(markers []string) int {
	for _, m := range markers {
		if n, ok := strings.CutPrefix(m, "pid="); ok {
			pid := 0
			for _, c := range n {
				pid = pid*10 + int(c-'0')
			}
			return pid
		}
	}
	return 0
}

// gone reports whether pid is no process within a short grace period.
func gone(pid int) bool {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
	}
	return false
}

func (s *LauncherSignalTestSuite) TestEveryLauncherStopsCleanly(t *gotest.T) {
	t.When("a run is stopped by a signal, however gotest was launched", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []launcherRow{
			{Desc: "direct, SIGINT: the fixture is released, exit 130, no process outlives the run", launcher: "direct", signal: "SIGINT"},
			{Desc: "direct, SIGTERM: the fixture is released, exit 130, no process outlives the run", launcher: "direct", signal: "SIGTERM"},
			{Desc: "direct, a second SIGINT: the teardown is abandoned, exit 130, no process outlives the run", launcher: "direct", signal: "second"},
			{Desc: "go run, SIGINT: the fixture is released, go run reports exit status 130, no process outlives the run", launcher: "go run", signal: "SIGINT"},
			{Desc: "go run, SIGTERM: the fixture is released, go run reports exit status 130, no process outlives the run", launcher: "go run", signal: "SIGTERM"},
			{Desc: "go run, a second SIGINT: the teardown is abandoned, go run reports exit status 130, no process outlives the run", launcher: "go run", signal: "second"},
			{Desc: "go tool, SIGINT: the fixture is released, exit 130, no process outlives the run", launcher: "go tool", signal: "SIGINT"},
			{Desc: "go tool, SIGTERM: the fixture is released, exit 130, no process outlives the run", launcher: "go tool", signal: "SIGTERM"},
			{Desc: "go tool, a second SIGINT: the teardown is abandoned, exit 130, no process outlives the run", launcher: "go tool", signal: "second"},
		}) {
			markerDir := it.TempDir()
			cmd := s.command(row.launcher)
			extra := []string{}
			if row.signal == "second" {
				extra = append(extra, "STOPPING_SLOW=1")
			}
			cmd.Env = childEnv(markerDir, extra...)
			tree := holding(it, cmd, markerDir)
			deliver(it, cmd, tree, row.signal)
			run := finished(cmd, tree, markerDir)

			if row.launcher == "go run" {
				gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(it, run.Stderr, "exit status 130")
			} else {
				gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			}
			if row.signal == "second" {
				gotest.NotContains(it, run.Markers, "teardown done")
			} else {
				gotest.Contains(it, run.Markers, "teardown done")
			}
			pid := pidOf(run.Markers)
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		}
	})
}
