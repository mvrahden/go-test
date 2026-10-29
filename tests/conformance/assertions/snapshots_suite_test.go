package assertions_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotKeysTestSuite: a snapshot section is the exact name testing gave
// the subtest, found beside the test whatever the build flags.
type SnapshotKeysTestSuite struct{}

func (s *SnapshotKeysTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *SnapshotKeysTestSuite) TestKeys(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/")
	gotest.NoError(t, err)
	t.When("Each rows and same-named subtests match snapshots", func(w *gotest.T) {
		w.It("#0, #1, x and x#01 each read their own section", func(it *gotest.T) {
			for _, name := range []string{"TestRows/#0", "TestRows/#1", "TestTwins/x", "TestTwins/x#01"} {
				gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/"+name), "stdout: %s", run.Stdout)
			}
		})
	})
}

func (s *SnapshotKeysTestSuite) TestLineEnds(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/")
	gotest.NoError(t, err)
	t.When("a value ends lines with runs of carriage returns, or ends in one", func(w *gotest.T) {
		w.It("matches the baseline written with plain newlines", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestCarriageReturns"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *SnapshotKeysTestSuite) TestTrimpath(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/", "-trimpath")
	gotest.NoError(t, err)
	t.When("the package is built with -trimpath", func(w *gotest.T) {
		w.It("the baselines are still found beside the test, _ext suffix included", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestTwins/x#01"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestRows/#1"))
		})
	})
}

func (s *SnapshotKeysTestSuite) TestUpdateSwitchOff(t *gotest.T) {
	// A staged copy: a switch that is not off rewrites this baseline, never
	// the committed one the other methods read.
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snapkeys"), staged, "snapmod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapKeysTestSuite_ext.snap")
	before, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1", "GOTEST_UPDATE_SNAPSHOTS=0"}, "-json", "./snapkeys/")
	gotest.NoError(t, err)
	after, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	t.When("GOTEST_UPDATE_SNAPSHOTS is 0", func(w *gotest.T) {
		w.It("a stale baseline still fails and is left as it was", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"), "stdout: %s", run.Stdout)
			gotest.Equal(it, string(before), string(after), "the baseline was rewritten")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestSwitchSpellings(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snapkeys"), staged, "snapspellmod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapKeysTestSuite_ext.snap")
	without := strings.Replace(readFile(t, baseline), "=== SNAP TestRows/#1 ===\nrow one\n", "", 1)
	gotest.NoError(t, os.WriteFile(baseline, []byte(without), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=FALSE", "GOTEST_UPDATE_SNAPSHOTS=f"}, "-json", "./snapkeys/")
	gotest.NoError(t, err)
	after := readFile(t, baseline)
	t.When("GOTEST_CI and GOTEST_UPDATE_SNAPSHOTS are spelled FALSE and f", func(w *gotest.T) {
		w.It("both are off: a missing baseline is written and a stale one still fails", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestRows/#1"), "stdout: %s", run.Stdout)
			gotest.Contains(it, after, "=== SNAP TestRows/#1 ===\nrow one\n")
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"))
			gotest.Contains(it, after, "=== SNAP TestStale ===\nold\n")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestNames(t *gotest.T) {
	bad := []string{"two\nlines", "=== SNAP forged ==="}
	results := make([]*gotest.R, len(bad))
	for i, name := range bad {
		results[i] = gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "v", name) })
	}
	written := filepath.Join("testdata", "__snapshots__", "unknown_ext.snap")
	_, err := os.Stat(written)
	// Whatever a regression wrote beside this suite; Remove leaves a dir
	// that holds anything else.
	for _, p := range []string{written, filepath.Dir(written), "testdata"} {
		_ = os.Remove(p)
	}
	t.When("a snapshot name spans lines or spells a section header", func(w *gotest.T) {
		w.It("is refused before anything is written", func(it *gotest.T) {
			for i, m := range results {
				gotest.True(it, m.Failed(), "name %q was accepted", bad[i])
				gotest.Contains(it, m.Message(), "snapshot name")
			}
			gotest.True(it, os.IsNotExist(err), "a refused name still wrote a file")
		})
	})
}

// readFile reads a file of a staged fixture, failing t when it cannot.
func readFile(t *gotest.T, path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	return string(data)
}

func (s *SnapshotKeysTestSuite) TestLegacyKeys(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snaplegacy"), staged, "snaplegacymod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapLegacyTestSuite_ext.snap")
	before := readFile(t, baseline)
	ci, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1"}, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	afterCI := readFile(t, baseline)
	local, err := testkit.RunCLIWithEnv(t.Context(), staged, nil, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	afterLocal := readFile(t, baseline)
	again, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1"}, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	verdict := func(r testkit.Run, name string) string { return r.Verdict("TestSnapLegacyTestSuite/" + name) }
	t.When("a baseline is keyed the way the release before exact names keyed it", func(w *gotest.T) {
		w.It("its content is compared under the new key", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict(ci, "TestIssue/issue_#42"), "stdout: %s", ci.Stdout)
			gotest.Equal(it, "pass", verdict(ci, "TestRows/#0"))
			gotest.Equal(it, "fail", verdict(ci, "TestStale/issue_#7"))
			gotest.Equal(it, "fail", verdict(local, "TestStale/issue_#7"))
		})
		w.It("under GOTEST_CI the file is left as it was", func(it *gotest.T) {
			gotest.Equal(it, before, afterCI)
		})
		w.It("otherwise a matching section is renamed to the new key", func(it *gotest.T) {
			gotest.Contains(it, afterLocal, "=== SNAP TestIssue/issue_#42 ===\nforty-two\n")
			gotest.Contains(it, afterLocal, "=== SNAP TestRows/#0 n ===\nrow zero\n")
			gotest.NotContains(it, afterLocal, "=== SNAP TestIssue/issue_ ===")
			gotest.NotContains(it, afterLocal, "=== SNAP TestRows/ n ===")
			gotest.Equal(it, "pass", verdict(again, "TestIssue/issue_#42"), "stdout: %s", again.Stdout)
			gotest.Equal(it, "pass", verdict(again, "TestRows/#0"))
		})
	})
	t.When("the old key of a section is the exact key of another test", func(w *gotest.T) {
		w.It("the section is not reused for a second key", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict(ci, "TestTwins/x"))
			gotest.Equal(it, "fail", verdict(ci, "TestTwins/x#01"), "x#01 read the section x claims")
			gotest.Equal(it, "pass", verdict(local, "TestTwins/x#01"))
			gotest.Contains(it, afterLocal, "=== SNAP TestTwins/x ===\nfirst twin\n=== SNAP TestTwins/x#01 ===\nsecond twin\n")
		})
	})
}
