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
	// Estimated for Windows CI: slowest method ~75s, setup ~45s; budgets are twice that, rounded up.
	return gotest.SuiteConfig{Timeout: 3 * time.Minute, SetupTimeout: 90 * time.Second}
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
	// An -exec wrapper that stays the suite's parent: the converter's death
	// reaches neither, as where no death signal comes.
	gotest.NoError(t, os.WriteFile(filepath.Join(s.staged, "wrap.sh"), []byte("#!/bin/sh\n\"$@\"\nexit $?\n"), 0o700)) //nolint:gosec // G306: the wrapper must be executable
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
	launcher string // direct, exec wrapper, go run, go tool
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
	case "exec wrapper":
		cmd = exec.Command(s.cli, "-json", "-exec", filepath.Join(s.staged, "wrap.sh"), "./stopping/") //nolint:gosec // G204: binary built by this suite
	default:
		cmd = exec.Command(s.cli, "-json", "./stopping/") //nolint:gosec // G204: binary built by this suite
	}
	cmd.Dir = s.staged
	return cmd
}

// deliver sends the row's signal to the run's process group, as a terminal
// (SIGINT) or a job supervisor (SIGTERM) would; "second" is Ctrl-C twice,
// the second landing while the fixture's teardown runs.
func deliver(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, markerDir, signal string) {
	group := -cmd.Process.Pid
	switch signal {
	case "SIGTERM":
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGTERM))
	case "second":
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGINT))
		secondCtrlC(t, cmd, tree, time.Now(), markerDir, "fixture.AfterAll")
	default:
		signalled(t, cmd, tree, syscall.Kill(group, syscall.SIGINT))
	}
}

// secondCtrlC sends the run's process group a second SIGINT once every marker
// in wants is in markerDir, and no sooner than a person's second Ctrl-C
// after the first, sent at first: a repeat sooner is the first delivered
// again.
func secondCtrlC(t *gotest.T, cmd *exec.Cmd, tree *proctree.Tree, first time.Time, markerDir string, wants ...string) {
	awaitMarkers(markerDir, 60*time.Second, wants...)
	time.Sleep(time.Until(first.Add(600 * time.Millisecond)))
	signalled(t, cmd, tree, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT))
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

// gone reports whether the suite process is no process within a short grace
// period. The pid itself is probed: the suite process leads no group of its
// own, so a probe of the group it would lead finds none, whatever runs.
func gone(pid int) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
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
		w.It("ends with its exit code, or by the signal where go run dies of a SIGTERM, finishes the fixture teardown unless a second signal abandons it saying so, and leaves no suite process behind", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []launcherRow{
				{Desc: "direct, SIGINT", launcher: "direct", signal: "SIGINT"},
				{Desc: "direct, SIGTERM", launcher: "direct", signal: "SIGTERM"},
				{Desc: "direct, a second SIGINT", launcher: "direct", signal: "second"},
				{Desc: "under an -exec wrapper, SIGINT", launcher: "exec wrapper", signal: "SIGINT"},
				{Desc: "under an -exec wrapper, a second SIGINT", launcher: "exec wrapper", signal: "second"},
				{Desc: "go run, SIGINT", launcher: "go run", signal: "SIGINT"},
				{Desc: "go run, SIGTERM", launcher: "go run", signal: "SIGTERM"},
				{Desc: "go run, a second SIGINT", launcher: "go run", signal: "second"},
				{Desc: "go tool, SIGINT", launcher: "go tool", signal: "SIGINT"},
				{Desc: "go tool, SIGTERM", launcher: "go tool", signal: "SIGTERM"},
				{Desc: "go tool, a second SIGINT", launcher: "go tool", signal: "second"},
			}) {
				markerDir := sub.TempDir()
				cmd := s.command(row.launcher)
				// A teardown slow enough to show as one when nobody waits
				// for it, and to outlast the second signal's kill.
				slow := "STOPPING_SLOW=2s"
				if row.signal == "second" {
					slow = "STOPPING_SLOW=6s"
				}
				cmd.Env = testkit.ChildEnv(markerDir, slow)
				tree := holding(sub, cmd, markerDir)
				deliver(sub, cmd, tree, markerDir, row.signal)
				run := finished(cmd, tree, markerDir)

				switch {
				case row.launcher == "go run" && row.signal == "SIGTERM":
					gotest.Equal(sub, -1, run.Exit, "go run ends by the signal; stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				case row.launcher == "go run":
					gotest.Equal(sub, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
					gotest.Contains(sub, run.Stderr, "exit status 130")
				default:
					gotest.Equal(sub, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				}
				if row.signal == "second" {
					gotest.Contains(sub, run.Stderr, "interrupted again")
					gotest.NotContains(sub, run.Markers, "teardown done")
				} else {
					gotest.NotContains(sub, run.Stderr, "interrupted again")
					gotest.Contains(sub, run.Markers, "teardown done")
				}
				pid := pidOf(run.Markers)
				gotest.NotZero(sub, pid, "markers: %v", run.Markers)
				gotest.True(sub, gone(pid), "the suite process %d outlived the run", pid)
			}
		})
	})
}

func (s *LauncherSignalTestSuite) TestASecondCtrlCReleasesTheFixturesAtOnce(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := s.command("direct")
	cmd.Env = testkit.ChildEnv(markerDir, "STOPPING_IGNORE_CONTEXT=1")
	tree := holding(t, cmd, markerDir)
	first := time.Now()
	signalled(t, cmd, tree, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT))
	// The method ignores the stop: the grace it is given outlasts the
	// second signal by seconds.
	secondCtrlC(t, cmd, tree, first, markerDir)
	run := finished(cmd, tree, markerDir)

	t.When("a second SIGINT reaches gotest while a stopped method body ignores its context", func(w *gotest.T) {
		w.It("releases the fixture at once, without AfterAll", func(it *gotest.T) {
			gotest.Contains(it, run.Markers, "teardown done", "stderr: %s", run.Stderr)
			gotest.NotContains(it, run.Markers, "suite.AfterAll")
		})
		w.It("exits 130 and leaves no suite process behind", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "interrupted again")
			pid := pidOf(run.Markers)
			gotest.NotZero(it, pid, "markers: %v", run.Markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		})
	})
}
