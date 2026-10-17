package gotestbench_test

import (
	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EnvTestSuite tests the environment check that tells a run its baseline was
// recorded somewhere else.
type EnvTestSuite struct{}

func (s *EnvTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func mkEnv(goos, goarch, goVersion string) gotestbench.Baseline {
	return gotestbench.Baseline{GOOS: goos, GOARCH: goarch, GoVersion: goVersion}
}

func (s *EnvTestSuite) TestEnvMismatch(t *gotest.T) {
	t.When("both sides record the same environment", func(w *gotest.T) {
		w.It("finds nothing", func(it *gotest.T) {
			diffs := gotestbench.EnvMismatch(
				mkEnv("linux", "amd64", "go1.27.0"),
				mkEnv("linux", "amd64", "go1.27.0"),
			)
			gotest.Empty(it, diffs)
		})
	})

	t.When("the baseline came from another machine", func(w *gotest.T) {
		w.It("names the cpu and GOMAXPROCS that differ", func(it *gotest.T) {
			old := mkEnv("linux", "amd64", "go1.27.0")
			old.CPU, old.GOMAXPROCS = "Example CPU A", 4
			now := mkEnv("linux", "amd64", "go1.27.0")
			now.CPU, now.GOMAXPROCS = "Example CPU B", 8
			gotest.Equal(it, []gotestbench.EnvDiff{
				{Field: "cpu", Baseline: "Example CPU A", Run: "Example CPU B"},
				{Field: "gomaxprocs", Baseline: "4", Run: "8"},
			}, gotestbench.EnvMismatch(old, now))
		})

		w.It("treats a baseline that recorded neither as unknown, not different", func(it *gotest.T) {
			now := mkEnv("linux", "amd64", "go1.27.0")
			now.CPU, now.GOMAXPROCS = "Example CPU B", 8
			gotest.Empty(it, gotestbench.EnvMismatch(mkEnv("linux", "amd64", "go1.27.0"), now))
		})
	})

	t.When("the baseline came from another platform and toolchain", func(w *gotest.T) {
		w.It("names every field that differs, baseline value first", func(it *gotest.T) {
			diffs := gotestbench.EnvMismatch(
				mkEnv("darwin", "arm64", "go1.26.0"),
				mkEnv("linux", "amd64", "go1.27.0"),
			)
			gotest.Equal(it, []gotestbench.EnvDiff{
				{Field: "goos", Baseline: "darwin", Run: "linux"},
				{Field: "goarch", Baseline: "arm64", Run: "amd64"},
				{Field: "goVersion", Baseline: "go1.26.0", Run: "go1.27.0"},
			}, diffs)
		})
	})

	t.When("only the toolchain moved", func(w *gotest.T) {
		w.It("reports that field alone", func(it *gotest.T) {
			diffs := gotestbench.EnvMismatch(
				mkEnv("linux", "amd64", "go1.26.0"),
				mkEnv("linux", "amd64", "go1.27.0"),
			)
			gotest.Len(it, diffs, 1)
			gotest.Equal(it, "goVersion", diffs[0].Field)
		})
	})

	// A pre-1.30 baseline, or one hand-written for a fixture, may leave the
	// platform fields out. Unknown is not a difference.
	t.When("the baseline left a field empty", func(w *gotest.T) {
		w.It("treats it as unknown, not as a mismatch", func(it *gotest.T) {
			diffs := gotestbench.EnvMismatch(
				mkEnv("", "", ""),
				mkEnv("linux", "amd64", "go1.27.0"),
			)
			gotest.Empty(it, diffs)
		})
	})
}

func (s *EnvTestSuite) TestFormatEnvMismatch(t *gotest.T) {
	t.When("nothing differs", func(w *gotest.T) {
		w.It("renders no line at all", func(it *gotest.T) {
			gotest.Empty(it, gotestbench.FormatEnvMismatch(nil))
		})
	})

	t.When("two fields differ", func(w *gotest.T) {
		w.It("names both and says what it means for the deltas", func(it *gotest.T) {
			note := gotestbench.FormatEnvMismatch([]gotestbench.EnvDiff{
				{Field: "goos", Baseline: "darwin", Run: "linux"},
				{Field: "goarch", Baseline: "arm64", Run: "amd64"},
			})
			gotest.Equal(it,
				"baseline was recorded in a different environment "+
					"(goos darwin, now linux; goarch arm64, now amd64); "+
					"the deltas compare runs that are not alike",
				note)
		})
	})
}
