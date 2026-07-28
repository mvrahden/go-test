package gotestbench_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BaselineTestSuite tests baseline extraction from spec trees and its
// JSON persistence (Save/Load round trip).
type BaselineTestSuite struct{}

func (s *BaselineTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// buildTree parses a go test -json stream (one JSON object per line) into a
// spec tree, mirroring how cmd/gotest builds trees from captured JSON.
func buildTree(it *gotest.T, ndjson string) []*gotestspec.Package {
	events, err := gotestspec.ParseEvents(bytes.NewReader([]byte(strings.TrimSpace(ndjson))))
	gotest.NoError(it, err)
	return gotestspec.BuildTree(events)
}

func (s *BaselineTestSuite) TestFromPackages(t *gotest.T) {
	t.When("a benchmark suite wrapper contains a leaf benchmark", func(w *gotest.T) {
		w.It("derives Package/Suite/Name and one Sample from the leaf's fields", func(it *gotest.T) {
			pkgs := buildTree(it, `
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t 1201 \t 985.2 ns/op \t 24 B/op \t 3 allocs/op\n"}
{"Action":"pass","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse","Elapsed":0.01}
{"Action":"pass","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite","Elapsed":0.01}
`)

			b := gotestbench.FromPackages(pkgs)

			gotest.Equal(it, 1, b.SchemaVersion)
			gotest.Len(it, b.Results, 1)

			r := b.Results[0]
			gotest.Equal(it, "example.com/pkg", r.Package)
			gotest.Equal(it, "FooTestSuite", r.Suite)
			gotest.Equal(it, "BenchmarkParse", r.Name)
			gotest.Len(it, r.Samples, 1)

			sample := r.Samples[0]
			gotest.Equal(it, 1201, sample.Iterations)
			gotest.InDelta(it, 985.2, sample.NsPerOp, 0.01)
			gotest.Equal(it, int64(24), sample.BytesPerOp)
			gotest.Equal(it, int64(3), sample.AllocsPerOp)
		})
	})

	t.When("a bare top-level benchmark has no enclosing suite wrapper", func(w *gotest.T) {
		w.It("reports an empty Suite", func(it *gotest.T) {
			pkgs := buildTree(it, `
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkStandalone"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkStandalone","Output":"BenchmarkStandalone-8   \t 500 \t 100.0 ns/op\n"}
{"Action":"pass","Package":"example.com/pkg","Test":"BenchmarkStandalone","Elapsed":0.01}
`)

			b := gotestbench.FromPackages(pkgs)

			gotest.Len(it, b.Results, 1)
			r := b.Results[0]
			gotest.Equal(it, "", r.Suite)
			gotest.Equal(it, "BenchmarkStandalone", r.Name)
			gotest.Len(it, r.Samples, 1)
		})
	})

	t.When("no benchmark leaves are present", func(w *gotest.T) {
		w.It("returns an empty Results slice", func(it *gotest.T) {
			b := gotestbench.FromPackages(nil)
			gotest.Empty(it, b.Results)
		})
	})

	t.When("a KindBenchmark leaf never recorded metrics (Iterations==0)", func(w *gotest.T) {
		w.It("is skipped rather than poisoning the baseline with a zero sample", func(it *gotest.T) {
			// A leaf can reach FromPackages with Iterations==0 when its bench
			// output line was never successfully parsed (e.g. a malformed or
			// truncated line). Recording it would corrupt Welch mean/variance
			// comparisons downstream, so it must be dropped, not sampled.
			pkgs := []*gotestspec.Package{
				{
					Path: "example.com/pkg",
					Nodes: []*gotestspec.Node{
						{
							Name: "BenchmarkFoo",
							Kind: gotestspec.KindBenchmark,
						},
					},
				},
			}

			b := gotestbench.FromPackages(pkgs)

			gotest.Empty(it, b.Results)
		})
	})

	t.When("a -count=4 run leaves repetitions 2..N as package-level output", func(w *gotest.T) {
		w.It("harvests all 4 repetitions into Samples on the matching Result", func(it *gotest.T) {
			// Mirrors real go test -json -count=4 output: only the first
			// repetition's line is tagged with the benchmark's Test field
			// (and gets attributed to the tree node); repetitions 2..N
			// arrive as untagged package-level "output" events carrying the
			// same "Benchmark<Suite>/Benchmark<Name>-<GOMAXPROCS>" prefix.
			pkgs := buildTree(it, `
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t    1200\t     980.0 ns/op\t      24 B/op\t       3 allocs/op\n"}
{"Action":"output","Package":"example.com/pkg","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t    1190\t     990.0 ns/op\t      24 B/op\t       3 allocs/op\n"}
{"Action":"output","Package":"example.com/pkg","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t    1210\t     970.0 ns/op\t      24 B/op\t       3 allocs/op\n"}
{"Action":"output","Package":"example.com/pkg","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t    1180\t    1000.0 ns/op\t      24 B/op\t       3 allocs/op\n"}
{"Action":"output","Package":"example.com/pkg","Output":"PASS\n"}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.02}
`)

			b := gotestbench.FromPackages(pkgs)

			gotest.Len(it, b.Results, 1)
			r := b.Results[0]
			gotest.Equal(it, "FooTestSuite", r.Suite)
			gotest.Equal(it, "BenchmarkParse", r.Name)
			gotest.Len(it, r.Samples, 4)

			gotest.Equal(it, 1200, r.Samples[0].Iterations)
			gotest.InDelta(it, 980.0, r.Samples[0].NsPerOp, 0.01)
			gotest.Equal(it, 1190, r.Samples[1].Iterations)
			gotest.InDelta(it, 990.0, r.Samples[1].NsPerOp, 0.01)
			gotest.Equal(it, 1210, r.Samples[2].Iterations)
			gotest.InDelta(it, 970.0, r.Samples[2].NsPerOp, 0.01)
			gotest.Equal(it, 1180, r.Samples[3].Iterations)
			gotest.InDelta(it, 1000.0, r.Samples[3].NsPerOp, 0.01)
		})
	})
}

func (s *BaselineTestSuite) TestSaveLoadRoundTrip(t *gotest.T) {
	t.When("a baseline is saved and reloaded", func(w *gotest.T) {
		w.It("preserves the JSON structure exactly", func(it *gotest.T) {
			dir := it.TempDir()
			path := filepath.Join(dir, "baseline.json")

			original := gotestbench.Baseline{
				SchemaVersion: 1,
				CreatedAt:     time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
				GoVersion:     "go1.24.0",
				GOOS:          "linux",
				GOARCH:        "amd64",
				Results: []gotestbench.Result{
					{
						Package: "example.com/pkg",
						Suite:   "FooTestSuite",
						Name:    "BenchmarkParse",
						Samples: []gotestbench.Sample{
							{Iterations: 1201, NsPerOp: 985.2, BytesPerOp: 24, AllocsPerOp: 3},
							{Iterations: 1180, NsPerOp: 990.1, BytesPerOp: 24, AllocsPerOp: 3},
						},
					},
				},
			}

			err := gotestbench.Save(path, original)
			gotest.NoError(it, err)

			loaded, err := gotestbench.Load(path)
			gotest.NoError(it, err)

			gotest.Equal(it, original.SchemaVersion, loaded.SchemaVersion)
			gotest.True(it, original.CreatedAt.Equal(loaded.CreatedAt))
			gotest.Equal(it, original.GoVersion, loaded.GoVersion)
			gotest.Equal(it, original.GOOS, loaded.GOOS)
			gotest.Equal(it, original.GOARCH, loaded.GOARCH)
			gotest.JSONEq(it, original.Results, loaded.Results)
		})

		w.It("rejects an unknown schema version", func(it *gotest.T) {
			dir := it.TempDir()
			path := filepath.Join(dir, "baseline.json")

			err := gotestbench.Save(path, gotestbench.Baseline{SchemaVersion: 2})
			gotest.NoError(it, err)

			_, err = gotestbench.Load(path)
			gotest.Error(it, err)
		})
	})
}
