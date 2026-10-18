// Package gotestbench provides a JSON baseline format for benchmark
// results extracted from gotestspec trees, and a Welch's t-test based
// significance comparison between two baselines.
package gotestbench

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// schemaVersion is the only Baseline.SchemaVersion this package writes and
// accepts on Load.
const schemaVersion = 1

// Baseline is a persisted snapshot of benchmark results, keyed by package,
// suite, and benchmark name.
type Baseline struct {
	SchemaVersion int       `json:"schemaVersion"` // 1
	CreatedAt     time.Time `json:"createdAt"`
	GoVersion     string    `json:"goVersion"`
	GOOS          string    `json:"goos"`
	GOARCH        string    `json:"goarch"`
	// CPU is the cpu line go test printed and GOMAXPROCS the suffix of the
	// result lines; both empty when the run did not say.
	CPU        string   `json:"cpu,omitempty"`
	GOMAXPROCS int      `json:"gomaxprocs,omitempty"`
	Results    []Result `json:"results"`
}

// Result is one benchmark's samples, identified by its package path, its
// enclosing suite name (empty for bare top-level benchmarks with no suite
// wrapper), and its path below the suite: the method name, extended by
// "/<name>" for each sub-benchmark b.Run opened.
type Result struct {
	Package string   `json:"package"`
	Suite   string   `json:"suite"`
	Name    string   `json:"name"`
	Samples []Sample `json:"samples"` // one per -count repetition (see FromPackages)
}

// Sample mirrors the fields go test prints per benchmark run:
// N   ns/op   B/op   allocs/op, plus MB/s and the metrics the benchmark
// reported, by unit.
type Sample struct {
	Iterations  int                `json:"iterations"`
	NsPerOp     float64            `json:"nsPerOp"`
	BytesPerOp  int64              `json:"bytesPerOp"`
	AllocsPerOp int64              `json:"allocsPerOp"`
	Metrics     map[string]float64 `json:"metrics,omitempty"`
}

// FromPackages walks the KindBenchmark leaves of pkgs and produces a
// Baseline.
//
// Key concept: Package is the Go package path, Suite is the bench wrapper's
// suite name (the top-level node name with its "Benchmark" prefix
// stripped), and Name is the leaf benchmark method's own node name. Bare
// top-level benchmarks with no enclosing suite wrapper get an empty Suite.
//
// Sample harvesting for -count=N: go test's own -json encoder only tags the
// first repetition's output line with the benchmark's Test field, so
// gotestspec.BuildTree attributes only that first repetition to the tree
// node. Repetitions 2..N arrive as untagged package-level output lines
// (routed by BuildTree into Package.Output) and are invisible to the tree
// walk above. FromPackages recovers them in a second pass: it scans each
// package's Output for lines matching the bench-line shape and matches
// them by their full benchmark name (e.g. "BenchmarkSuite/BenchmarkParse")
// to the Result a tree leaf already produced, appending one Sample per
// matched line. A `-count=4` run therefore produces `len(Samples)==4` per
// benchmark. Lines that don't match any known Result (e.g. stray output)
// are ignored.
func FromPackages(pkgs []*gotestspec.Package) Baseline {
	results := make([]Result, 0)
	index := make(map[string]int)
	var cpu string
	procs := 0

	for _, pkg := range pkgs {
		for _, top := range pkg.Nodes {
			collectBenchLeaves(pkg.Path, top, top, "", &results, index, &procs)
		}
	}

	for _, pkg := range pkgs {
		harvestPackageOutputSamples(pkg.Path, pkg.Output, &results, index)
		if cpu == "" {
			cpu = cpuLine(pkg.Output)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.Suite != b.Suite {
			return a.Suite < b.Suite
		}
		return a.Name < b.Name
	})

	return Baseline{
		SchemaVersion: schemaVersion,
		CreatedAt:     time.Now().UTC(),
		GoVersion:     runtime.Version(),
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		CPU:           cpu,
		GOMAXPROCS:    procs,
		Results:       results,
	}
}

// cpuLine finds the "cpu: <model>" line go test prints once per package.
func cpuLine(output []string) string {
	for _, line := range strings.Split(strings.Join(output, ""), "\n") {
		if model, ok := strings.CutPrefix(strings.TrimSpace(line), "cpu: "); ok {
			return strings.TrimSpace(model)
		}
	}
	return ""
}

// collectBenchLeaves recurses through n's subtree, recording a Sample for
// every KindBenchmark leaf (a benchmark node with no children). top is the
// package-level ancestor node used to derive Suite; when n is itself top
// (a bare top-level benchmark with no suite wrapper), Suite is left empty.
// below is the path from the suite down to n's parent; a leaf's Name is that
// path extended by its own, so a sub-benchmark is keyed by where it sits.
// procs takes the first GOMAXPROCS suffix a leaf carries.
func collectBenchLeaves(pkgPath string, top, n *gotestspec.Node, below string, results *[]Result, index map[string]int, procs *int) {
	name := n.Name
	if below != "" {
		name = below + "/" + n.Name
	}
	if n.Kind == gotestspec.KindBenchmark && len(n.Children) == 0 {
		// Defense in depth: a leaf can reach here with Iterations==0 if its
		// bench output line was never successfully parsed (e.g. split
		// across output events in a way even the joined scan in tree.go
		// missed, or a malformed line). Recording it would poison Welch
		// mean/variance comparisons downstream with a zero sample, so skip
		// it rather than sampling a benchmark that never actually ran.
		if n.Iterations == 0 {
			return
		}

		suite := ""
		if n != top {
			suite = strings.TrimPrefix(top.Name, "Benchmark")
		}
		if *procs == 0 {
			*procs = n.Procs
		}

		sample := Sample{
			Iterations:  n.Iterations,
			NsPerOp:     n.NsPerOp,
			BytesPerOp:  n.BytesPerOp,
			AllocsPerOp: n.AllocsPerOp,
			Metrics:     n.Metrics,
		}

		key := pkgPath + "\x00" + suite + "\x00" + name
		if idx, ok := index[key]; ok {
			(*results)[idx].Samples = append((*results)[idx].Samples, sample)
			return
		}
		index[key] = len(*results)
		*results = append(*results, Result{
			Package: pkgPath,
			Suite:   suite,
			Name:    name,
			Samples: []Sample{sample},
		})
		return
	}

	childBelow := ""
	if n != top {
		childBelow = name
	}
	for _, c := range n.Children {
		collectBenchLeaves(pkgPath, top, c, childBelow, results, index, procs)
	}
}

// harvestPackageOutputSamples scans a package's untagged Output lines for
// -count=N repetitions 2..N (see FromPackages) and appends a Sample to the
// matching Result in results/index for each one found. Lines that don't
// match the bench-line shape, or that match a name with no corresponding
// Result (e.g. incidental package output), are skipped.
//
// Package.Output entries are individual test2json "output" event payloads,
// not guaranteed to be line-aligned: under real subprocess I/O timing a
// benchmark result line can arrive split across two consecutive events.
// Joining them back into one stream and re-splitting on "\n" reconstructs
// complete lines before matching, regardless of how test2json chunked them.
func harvestPackageOutputSamples(pkgPath string, output []string, results *[]Result, index map[string]int) {
	lines := strings.Split(strings.Join(output, ""), "\n")
	for _, line := range lines {
		result, ok := gotestspec.ParseBenchLine(line)
		if !ok {
			continue
		}
		suite, name := splitBenchFullName(result.Name)
		key := pkgPath + "\x00" + suite + "\x00" + name
		idx, ok := index[key]
		if !ok {
			continue
		}
		(*results)[idx].Samples = append((*results)[idx].Samples, Sample{
			Iterations:  result.Iterations,
			NsPerOp:     result.NsPerOp,
			BytesPerOp:  result.BytesPerOp,
			AllocsPerOp: result.AllocsPerOp,
			Metrics:     result.Metrics,
		})
	}
}

// splitBenchFullName splits a benchmark's full name (as captured from a
// result line, e.g. "BenchmarkFooTestSuite/BenchmarkParse/sub") into the
// Suite and Name fields used to key a Result, mirroring collectBenchLeaves:
// Suite is the top-level segment with its "Benchmark" prefix stripped, and
// Name is everything below it. A name with no "/" is a bare top-level
// benchmark, reported with an empty Suite.
func splitBenchFullName(full string) (suite, name string) {
	top, rest, nested := strings.Cut(full, "/")
	if !nested {
		return "", top
	}
	return strings.TrimPrefix(top, "Benchmark"), rest
}

// Save writes b to path as indented JSON (0644).
func Save(path string, b Baseline) error { //nolint:gocritic // hugeParam: stable API
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("gotestbench: marshal baseline: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("gotestbench: write baseline: %w", err)
	}
	return nil
}

// Load reads and parses a Baseline from path. It rejects any
// SchemaVersion other than the one this package writes.
func Load(path string) (Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, fmt.Errorf("gotestbench: read baseline: %w", err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("gotestbench: parse baseline: %w", err)
	}
	if b.SchemaVersion != schemaVersion {
		return Baseline{}, fmt.Errorf("gotestbench: unsupported schema version %d (want %d)", b.SchemaVersion, schemaVersion)
	}
	return b, nil
}
