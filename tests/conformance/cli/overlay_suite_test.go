package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIOverlayTestSuite: the files a run generates are its own, so runs that
// share a cache cannot break each other's.
type CLIOverlayTestSuite struct{}

func (s *CLIOverlayTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *CLIOverlayTestSuite) TestConcurrentRunsSharingACache(t *gotest.T) {
	const runs = 16
	cache := t.TempDir()
	results := make([]testkit.Run, runs)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := range runs {
		markers := t.TempDir()
		wg.Go(func() {
			results[i], errs[i] = testkit.RunPackageWithEnv(t.Context(), markers, []string{protocol.EnvCacheDir + "=" + cache}, "../testdata/passing/")
		})
	}
	wg.Wait()

	t.When("16 runs of one package start at once and share one cache", func(w *gotest.T) {
		w.It("passes every run", func(it *gotest.T) {
			for i := range runs {
				gotest.NoError(it, errs[i])
				gotest.Equal(it, 0, results[i].Exit, "run %d stdout: %s\nstderr: %s", i, results[i].Stdout, results[i].Stderr)
			}
		})
	})
}

// tempEnv points a run's temp dir at dir, where its work dir goes.
func tempEnv(dir string) []string {
	return []string{"TMPDIR=" + dir, "TMP=" + dir, "TEMP=" + dir}
}

func (s *CLIOverlayTestSuite) TestARunsGeneratedFilesEndWithIt(t *gotest.T) {
	tmp := t.TempDir()
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), tempEnv(tmp), "../testdata/passing/")
	gotest.NoError(t, err)
	left, globErr := filepath.Glob(filepath.Join(tmp, "gotest-*"))

	t.When("a run ends", func(w *gotest.T) {
		w.It("leaves none of the files it generated behind", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.NoError(it, globErr)
			gotest.Empty(it, left)
		})
	})
}

func (s *CLIOverlayTestSuite) TestADebugRunKeepsItsGeneratedFiles(t *gotest.T) {
	tmp := t.TempDir()
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), tempEnv(tmp), "--debug", "../testdata/passing/")
	gotest.NoError(t, err)
	named := regexp.MustCompile(`DEBUG: overlay dir: (\S+)`).FindStringSubmatch(run.Stderr)

	t.When("a --debug run ends", func(w *gotest.T) {
		w.It("names its overlay on stderr and keeps it", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Len(it, named, 2, "stderr: %s", run.Stderr)
			_, statErr := os.Stat(filepath.Join(named[1], "overlay.json"))
			gotest.NoError(it, statErr)
		})
	})
}
