package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchCommandTestSuite drives 'gotest bench' through the built binary: its output, its baseline and the gate.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type BenchCommandTestSuite struct{ cli cliRunner }

func (s *BenchCommandTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *BenchCommandTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(t)
}

func (s *BenchCommandTestSuite) TestBenchDeltaLines(t *gotest.T) {
	t.When("first run", func(w *gotest.T) {
		w.It("prints no deltas but records ns/op", func(it *gotest.T) {
			results := []gotestbench.Result{
				{Package: "p", Suite: "Foo", Name: "BenchmarkBar", Samples: []gotestbench.Sample{{NsPerOp: 100}}},
			}
			lines, next := ExportBenchDeltaLines(results, nil)
			gotest.Empty(it, lines)
			gotest.Equal(it, 100.0, next["p\x00Foo\x00BenchmarkBar"])
		})
	})

	t.When("a benchmark regresses", func(w *gotest.T) {
		w.It("reports a positive delta", func(it *gotest.T) {
			results := []gotestbench.Result{
				{Package: "p", Suite: "Foo", Name: "BenchmarkBar", Samples: []gotestbench.Sample{{NsPerOp: 200}}},
			}
			prev := map[string]float64{"p\x00Foo\x00BenchmarkBar": 100}

			lines, next := ExportBenchDeltaLines(results, prev)

			gotest.Len(it, lines, 1)
			gotest.Contains(it, lines[0], "BenchmarkBar")
			gotest.Contains(it, lines[0], "200.00 ns/op")
			gotest.Contains(it, lines[0], "+100.0%")
			gotest.Equal(it, 200.0, next["p\x00Foo\x00BenchmarkBar"])
		})
	})

	t.When("a benchmark improves", func(w *gotest.T) {
		w.It("reports a negative delta", func(it *gotest.T) {
			results := []gotestbench.Result{
				{Package: "p", Suite: "Foo", Name: "BenchmarkBar", Samples: []gotestbench.Sample{{NsPerOp: 50}}},
			}
			prev := map[string]float64{"p\x00Foo\x00BenchmarkBar": 100}

			lines, _ := ExportBenchDeltaLines(results, prev)

			gotest.Len(it, lines, 1)
			gotest.Contains(it, lines[0], "-50.0%")
		})
	})

	t.When("a benchmark has multiple samples", func(w *gotest.T) {
		w.It("averages ns/op across samples", func(it *gotest.T) {
			results := []gotestbench.Result{
				{Package: "p", Suite: "", Name: "BenchmarkBaz", Samples: []gotestbench.Sample{{NsPerOp: 100}, {NsPerOp: 200}}},
			}
			_, next := ExportBenchDeltaLines(results, nil)
			gotest.Equal(it, 150.0, next["p\x00\x00BenchmarkBaz"])
		})
	})

	t.When("a benchmark is new this run", func(w *gotest.T) {
		w.It("prints no delta for it", func(it *gotest.T) {
			results := []gotestbench.Result{
				{Package: "p", Suite: "Foo", Name: "BenchmarkNew", Samples: []gotestbench.Sample{{NsPerOp: 100}}},
			}
			prev := map[string]float64{"p\x00Foo\x00BenchmarkOther": 50}

			lines, next := ExportBenchDeltaLines(results, prev)

			gotest.Empty(it, lines)
			gotest.Equal(it, 100.0, next["p\x00Foo\x00BenchmarkNew"])
		})
	})
}

func (s *BenchCommandTestSuite) TestBenchSubcommand(t *gotest.T) {
	t.It("runs suite benchmarks serially and prints ns/op lines", func(it *gotest.T) {
		out := s.cli.run(it, "bench", "./examples/notification", "-benchtime=10x")
		gotest.Contains(it, out, "BenchmarkNotificationDispatchBenchTestSuite")
		gotest.Contains(it, out, "ns/op")
	})
	t.When("running under GitHub Actions", func(w *gotest.T) {
		w.It("writes the benchmark results to the step summary", func(it *gotest.T) {
			summaryPath := filepath.Join(it.TempDir(), "summary.md")
			env := []string{"GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY=" + summaryPath}

			_, code := s.cli.runEnv(it, env, "bench", "--spec", "./examples/notification", "-benchtime=10x")
			gotest.Equal(it, 0, code)

			summary, err := os.ReadFile(summaryPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(summary), "### 1 benchmark ran (")
			gotest.Contains(it, string(summary), "| Benchmark | ns/op | B/op | allocs/op |")
			gotest.Contains(it, string(summary), "| NotificationDispatchBenchTestSuite/BenchmarkDispatch | ")
			gotest.NotContains(it, string(summary), "tests passed")
		})
	})
	t.It("reports when no benchmarks exist", func(it *gotest.T) {
		out := s.cli.run(it, "bench", "./internal/protocol")
		gotest.Contains(it, out, "no benchmarks found")
	})
}

func (s *BenchCommandTestSuite) TestBenchSaveAgainstGate(t *gotest.T) {
	t.It("saves a baseline with one Sample per -count repetition", func(it *gotest.T) {
		dir := it.TempDir()
		baselinePath := filepath.Join(dir, "baseline.json")

		out, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "-count=4", "--save="+baselinePath)
		gotest.Equal(it, 0, code)
		// --save implies spec rendering (Task 7's spec view), which trims
		// the suite wrapper's "TestSuite" suffix, so it shows up as
		// "BenchmarkNotificationDispatchBench" rather than the raw
		// "BenchmarkNotificationDispatchBenchTestSuite" wrapper name.
		gotest.Contains(it, out, "NotificationDispatchBench")
		gotest.Contains(it, out, "ns/op")

		data, err := os.ReadFile(baselinePath)
		gotest.NoError(it, err)
		var b gotestbench.Baseline
		gotest.NoError(it, json.Unmarshal(data, &b))
		gotest.NotEmpty(it, b.Results)
		for _, r := range b.Results {
			gotest.Len(it, r.Samples, 4)
		}
	})

	t.It("compares two saved baselines and passes an impossible-to-trip gate", func(it *gotest.T) {
		dir := it.TempDir()
		firstPath := filepath.Join(dir, "first.json")

		_, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "-count=6", "--save="+firstPath)
		gotest.Equal(it, 0, code)

		out, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "-count=6", "--against="+firstPath, "--gate=500")
		gotest.Equal(it, 0, code)
		// A bare --against (no --spec/--save) must still show the benchmark
		// actually ran, not just a delta table header: the tree's own
		// ns/op result line has to render alongside the comparison.
		gotest.Contains(it, out, "ns/op")
		gotest.Contains(it, out, "BENCHMARK")
		gotest.Contains(it, out, "OLD ns/op")
		gotest.Contains(it, out, "NEW ns/op")
	})

	t.It("errors when --gate is given without a baseline source", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "--gate=10")
		gotest.NotEqual(it, 0, code)
		gotest.Contains(it, out, "--gate requires --against")
	})

	t.It("renders exactly one summary trailer for --spec --against", func(it *gotest.T) {
		dir := it.TempDir()
		firstPath := filepath.Join(dir, "first.json")

		_, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "-count=6", "--save="+firstPath)
		gotest.Equal(it, 0, code)

		out, code := s.cli.runExit(it, "bench", "./examples/notification", "-benchtime=10x", "-count=6", "--spec", "--against="+firstPath)
		gotest.Equal(it, 0, code)
		// The spec tree's own trailing counts line must appear exactly
		// once, with no second, stacked "N tests passed (...)" trailer
		// from a separate RenderSummary call. Benchmarks carry no
		// pass/fail verdicts, so this trailer ends after the counts.
		gotest.Contains(it, out, "1 suite, 1 benchmark")
		gotest.NotContains(it, out, "tests passed (")
	})
}
