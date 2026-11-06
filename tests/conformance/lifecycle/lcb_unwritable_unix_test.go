//go:build unix

package lifecycle_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// UnwritableStopFileTestSuite: a stop whose stop file cannot be written
// kills the suite process alone, so the run ends at once and its stream ends
// whole.
type UnwritableStopFileTestSuite struct {
	cli string
}

func (s *UnwritableStopFileTestSuite) SuiteGuard() string {
	if os.Geteuid() == 0 {
		return "a read-only directory does not keep root from writing"
	}
	return ""
}

// SuiteConfig: ParentGoneTestSuite's budgets, which build the same CLI and
// stop the same package; the method bounds its own wait at 20s.
func (s *UnwritableStopFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 45 * time.Second}
}

func (s *UnwritableStopFileTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *UnwritableStopFileTestSuite) AfterAll(t *gotest.T) {}

// tmpDirCtx is the directory a run puts its temporary files in, its work
// dir among them.
type tmpDirCtx struct{ dir string }

func (s *UnwritableStopFileTestSuite) BeforeEach(t *gotest.T) *tmpDirCtx {
	return &tmpDirCtx{dir: t.TempDir()}
}

// AfterEach makes the work dir writable again, so it can be removed.
func (s *UnwritableStopFileTestSuite) AfterEach(t *gotest.T, ctx *tmpDirCtx) {
	dirs, _ := filepath.Glob(filepath.Join(ctx.dir, "gotest-work-*"))
	for _, dir := range dirs {
		_ = chmodDirs(dir, 0o700)
	}
}

// chmodDirs sets the mode of dir and every directory below it.
func chmodDirs(dir string, mode os.FileMode) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return os.Chmod(path, mode) //nolint:gosec // G122: the run's work dir, inside the test's own temp dir
	})
}

func (s *UnwritableStopFileTestSuite) TestTheWorkDirTurnedReadOnly(t *gotest.T, ctx *tmpDirCtx) {
	markerDir := t.TempDir()
	cmd := commandOf(s.cli, "-json", "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir, "TMPDIR="+ctx.dir, "GOTMPDIR="+ctx.dir)
	tree := started(t, cmd, markerDir, "running")
	workDirs, _ := filepath.Glob(filepath.Join(ctx.dir, "gotest-work-*"))
	readOnly := len(workDirs) == 1 && chmodDirs(workDirs[0], 0o500) == nil
	stopAt := time.Now()
	if readOnly {
		signalled(t, cmd, tree, testkit.Interrupt(cmd, tree))
	}
	// Well inside the 30s default deadline, which would end the method too.
	run := ended(cmd, tree, markerDir, 20*time.Second)
	took := time.Since(stopAt)
	gotest.True(t, readOnly, "the run's work dir could not be made read-only: %v", workDirs)

	t.When("the run is interrupted while a method holds and the suite's stop file cannot be written", func(w *gotest.T) {
		w.It("kills the suite process alone, saying on stderr that the stop file could not be written", func(it *gotest.T) {
			gotest.Regexp(it, `(?m)^WARN: \S+ TestStopTestSuite: the stop file could not be written \(open \S+\.stop: permission denied\), so the suite process was killed$`, run.Stderr)
			gotest.NotContains(it, run.Markers, "context ended", "the suite was stopped, not killed")
			gotest.Equal(it, "fail", packageVerdict(&run, stoppingPkg), "stream:\n%s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("ends at once, the run booked as interrupted on its stream", func(it *gotest.T) {
			gotest.Less(it, took, 10*time.Second, "stderr: %s", run.Stderr)
			gotest.Equal(it, "fail", packageVerdict(&run, "interrupted"), "stream:\n%s", run.Stdout)
			gotest.Contains(it, packageOutput(&run, "interrupted"), "TestStopTestSuite")
		})
	})
}

// stoppingPkg is the stopping package's import path.
const stoppingPkg = "github.com/mvrahden/go-test/tests/conformance/testdata/stopping"
