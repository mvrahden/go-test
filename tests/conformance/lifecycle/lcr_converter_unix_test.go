//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConverterEndsEarlyTestSuite: a converter that dies while the suite runs is
// replaced by a conversion of everything the suite wrote.
type ConverterEndsEarlyTestSuite struct {
	cli string
}

func (s *ConverterEndsEarlyTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *ConverterEndsEarlyTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *ConverterEndsEarlyTestSuite) AfterAll(t *gotest.T) {}

// converterOf finds the converter among the descendants of the process pid:
// the one whose command is test2json, 0 when there is none.
func converterOf(pid int) int {
	for queue := []int{pid}; len(queue) > 0; queue = queue[1:] {
		out, _ := exec.Command("pgrep", "-P", strconv.Itoa(queue[0])).Output() //nolint:gosec // G204: a pid of this test's own run
		for _, field := range strings.Fields(string(out)) {
			child, err := strconv.Atoi(field)
			if err != nil {
				continue
			}
			comm, _ := exec.Command("ps", "-o", "comm=", "-p", field).Output() //nolint:gosec // G204: a pid of this test's own run
			if filepath.Base(strings.TrimSpace(string(comm))) == "test2json" {
				return child
			}
			queue = append(queue, child)
		}
	}
	return 0
}

func (s *ConverterEndsEarlyTestSuite) TestAConverterKilledMidRun(t *gotest.T) {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "-run=^TestWaitTestSuite$", "../testdata/lcrrelay/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir, "running")
	converter := converterOf(cmd.Process.Pid)
	if converter == 0 {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		gotest.Fail(t, "no converter under the CLI") //nolint:fail-guard // the tree is killed before the test fails
	}
	// A child of the CLI this test started.
	signalled(t, cmd, tree, syscall.Kill(converter, syscall.SIGKILL))
	reaped := testkit.EndedWithin(converter, 5*time.Second)
	// The suite writes the rest of its stream only now.
	gotest.NoError(t, os.WriteFile(filepath.Join(markerDir, "release"), nil, 0o600))
	run := ended(cmd, tree, markerDir, 60*time.Second)

	t.When("the converter is killed while a suite runs", func(w *gotest.T) {
		w.It("converts the suite's output again, every verdict in the stream", func(it *gotest.T) {
			gotest.True(it, reaped, "the converter %d outlived its kill", converter)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			for _, path := range []string{
				"TestWaitTestSuite/TestWaits",
				"TestWaitTestSuite/TestAfter/passes_once_released",
				"TestWaitTestSuite/TestAfter",
				"TestWaitTestSuite/TestRows/first",
				"TestWaitTestSuite/TestRows/second",
				"TestWaitTestSuite/TestRows",
				"TestWaitTestSuite",
				"-",
			} {
				gotest.Equal(it, "pass", run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
			}
			gotest.Contains(it, outputOf(&run, "TestWaitTestSuite/TestWaits"), "released")
		})
		w.It("says so on stderr, naming the suite and how the converter ended", func(it *gotest.T) {
			gotest.Regexp(it, `gotest: \S+/lcrrelay TestWaitTestSuite: the converter ended early \(signal: killed\); the suite's output was converted again`, run.Stderr)
		})
	})
}
