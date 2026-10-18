package fixtures_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureCancelTestSuite: a shared fixture whose setup ends before its
// BeforeAll runs has nothing to release.
type FixtureCancelTestSuite struct {
	cli string
}

func (s *FixtureCancelTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FixtureCancelTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *FixtureCancelTestSuite) AfterAll(t *gotest.T) {}

// interruptedAt runs pkg, interrupts it once the marker is stamped and reads
// back what the run left. A run that never stamps it is killed before the
// test fails, so nothing outlives it.
func interruptedAt(t *gotest.T, cli, pkg, marker string) testkit.Run {
	markerDir := t.TempDir()
	cmd := exec.Command(cli, "-json", pkg) //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, marker) {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never reached %s:\n%s%s", marker, stdout, stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
	if err := testkit.Interrupt(cmd, tree); err != nil {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		gotest.Fail(t, "interrupt: %v", err) //nolint:fail-guard // the tree is killed before the test fails
	}
	_ = cmd.Wait()
	tree.Release()
	run := testkit.Run{Exit: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String()}
	run.Markers, _ = testkit.ReadMarkers(markers)
	return run
}

func (s *FixtureCancelTestSuite) TestSetupEndsBeforeBeforeAll(t *gotest.T) {
	t.When("a run is interrupted while a shared fixture's dependency sets up, before the fixture's own BeforeAll runs", func(w *gotest.T) {
		run := interruptedAt(w, s.cli, "../testdata/fixacancelshared/", "hold.BeforeAll")
		w.It("runs no AfterAll for the fixture, and the dependency's AfterAll", func(it *gotest.T) {
			gotest.Equal(it, []string{"hold.BeforeAll", "hold.AfterAll"}, run.Markers, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}
