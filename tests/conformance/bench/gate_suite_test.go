package bench_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchGateTestSuite: a comparison is against the baseline that was there
// before the run, never against what the run just wrote; a gate with nothing
// to compare is a failure, not a pass; and a gate needs enough samples for a
// significance test, where a plain comparison only marks the rows it lacks
// them for.
type BenchGateTestSuite struct{}

func (s *BenchGateTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// foreignBaseline is a baseline whose only benchmark lives in a package this
// run does not have.
func foreignBaseline(t *gotest.T) string {
	path := filepath.Join(t.TempDir(), "baseline.json")
	doc := map[string]any{
		"schemaVersion": 1, "createdAt": "2026-01-01T00:00:00Z",
		"goVersion": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"results": []map[string]any{{
			"package": "example.com/elsewhere", "suite": "OtherTestSuite", "name": "BenchmarkGone",
			"samples": []map[string]any{{"iterations": 1, "nsPerOp": 10.0, "bytesPerOp": 0, "allocsPerOp": 0}},
		}},
	}
	data, err := json.Marshal(doc)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func (s *BenchGateTestSuite) TestSaveAndAgainstNamingOneFile(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "same.json")
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "--save="+path, "--against="+path, "../testdata/benching/", "-benchtime=1x")
	gotest.NoError(t, err)
	t.When("--save and --against name the same file", func(w *gotest.T) {
		w.It("is refused before anything runs, since the run would be compared against itself", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "same file")
		})
	})
}

func (s *BenchGateTestSuite) TestAGateWithNothingToCompare(t *gotest.T) {
	baseline := foreignBaseline(t)
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--json", "--against="+baseline, "--gate=5", "../testdata/benching/", "-benchtime=1x", "-count=4")
	gotest.NoError(t, err)
	var report struct {
		Missing []string `json:"missing"`
		Added   []string `json:"added"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &report), "stdout is not a report:\n%s", run.Stdout)
	t.When("no benchmark is in both the baseline and the run", func(w *gotest.T) {
		w.It("fails the gate and says there was nothing to compare", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "nothing to compare")
		})
		w.It("lists the baseline's benchmarks that did not run and the run's that the baseline lacks", func(it *gotest.T) {
			gotest.Equal(it, []string{"example.com/elsewhere OtherTestSuite/BenchmarkGone"}, report.Missing)
			gotest.Len(it, report.Added, 1)
			gotest.Regexp(it, `JoinTestSuite/BenchmarkJoin$`, report.Added[0])
			gotest.Contains(it, run.Stderr, "BenchmarkGone")
		})
	})
}

// ownBaseline saves this run's own benchmarks as a baseline.
func ownBaseline(t *gotest.T, count string) string {
	path := filepath.Join(t.TempDir(), "baseline.json")
	run, err := testkit.RunCLI(t.Context(), ".", "bench", "--save="+path, "../testdata/benching/", "-benchtime=1x", "-count="+count)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	return path
}

func (s *BenchGateTestSuite) TestAGateNeedsSamples(t *gotest.T) {
	baseline := ownBaseline(t, "4")
	t.When("--gate is given with -count below 4", func(w *gotest.T) {
		for run, count := range gotest.Each(w, []struct {
			Desc string
			args []string
		}{
			{"-count absent", nil},
			{"-count=2", []string{"-count=2"}},
		}) {
			args := append([]string{"bench", "--against=" + baseline, "--gate=10", "../testdata/benching/", "-benchtime=1x"}, count.args...)
			res, err := testkit.RunCLI(run.Context(), ".", args...)
			gotest.NoError(run, err)
			run.It("exits 2 naming -count=4 as the requirement", func(it *gotest.T) {
				gotest.Equal(it, 2, res.Exit, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
				gotest.Contains(it, res.Stderr, "--gate needs -count=4 or more")
			})
		}
	})
	t.When("--gate is given with -count=4", func(w *gotest.T) {
		res, err := testkit.RunCLI(w.Context(), ".", "bench", "--against="+baseline, "--gate=1000", "../testdata/benching/", "-benchtime=1x", "-count=4")
		gotest.NoError(w, err)
		w.It("judges the run", func(it *gotest.T) {
			gotest.Equal(it, 0, res.Exit, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			gotest.NotContains(it, res.Stderr, "-count=4 or more")
		})
	})
}

func (s *BenchGateTestSuite) TestAComparisonWithFewSamples(t *gotest.T) {
	baseline := ownBaseline(t, "1")
	res, err := testkit.RunCLI(t.Context(), ".", "bench", "--against="+baseline, "../testdata/benching/", "-benchtime=1x", "-v")
	gotest.NoError(t, err)
	t.When("only --against is given and a side has fewer than 4 samples", func(w *gotest.T) {
		w.It("still compares and marks each such row as a ±20% heuristic", func(it *gotest.T) {
			gotest.Equal(it, 0, res.Exit, "stdout: %s\nstderr: %s", res.Stdout, res.Stderr)
			gotest.Regexp(it, `JoinTestSuite/BenchmarkJoin  [\d.]+  [\d.]+  [-+][\d.]+%( ⚠)? \(n<4, ±20% heuristic\)`, res.Stdout)
		})
	})
}
