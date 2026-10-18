package assertions_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotKeysTestSuite: a snapshot section is the exact name testing gave
// the subtest, found beside the test whatever the build flags.
type SnapshotKeysTestSuite struct{}

func (s *SnapshotKeysTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const snapkeysBaseline = "../testdata/snapkeys/testdata/__snapshots__/TestSnapKeysTestSuite_ext.snap"

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
	before, err := os.ReadFile(filepath.Clean(snapkeysBaseline))
	gotest.NoError(t, err)
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1", "GOTEST_UPDATE_SNAPSHOTS=0"}, "../testdata/snapkeys/")
	gotest.NoError(t, err)
	after, err := os.ReadFile(filepath.Clean(snapkeysBaseline))
	gotest.NoError(t, err)
	t.When("GOTEST_UPDATE_SNAPSHOTS is 0", func(w *gotest.T) {
		w.It("a stale baseline still fails and is left as it was", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"), "stdout: %s", run.Stdout)
			gotest.Equal(it, string(before), string(after), "the baseline was rewritten")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestNames(t *gotest.T) {
	t.When("a snapshot name spans lines or spells a section header", func(w *gotest.T) {
		w.It("is refused before anything is written", func(it *gotest.T) {
			for _, bad := range []string{"two\nlines", "=== SNAP forged ==="} {
				m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "v", bad) })
				gotest.True(it, m.Failed(), "name %q was accepted", bad)
				gotest.Contains(it, m.Message(), "snapshot name")
			}
			_, err := os.Stat(filepath.Join("testdata", "__snapshots__", "unknown_ext.snap"))
			gotest.True(it, os.IsNotExist(err), "a refused name still wrote a file")
		})
	})
}
