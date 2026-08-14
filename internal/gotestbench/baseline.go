// Package gotestbench provides a JSON baseline format for benchmark
// results extracted from gotestspec trees, and a Welch's t-test based
// significance comparison between two baselines.
package gotestbench

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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
	Results       []Result  `json:"results"`
}

// Result is one benchmark's samples, identified by its package path, its
// enclosing suite name (empty for bare top-level benchmarks with no suite
// wrapper), and its own method name.
type Result struct {
	Package string   `json:"package"`
	Suite   string   `json:"suite"`
	Name    string   `json:"name"`
	Samples []Sample `json:"samples"` // one per -count repetition (see FromPackages)
}

// Sample mirrors the fields go test prints per benchmark run:
// N   ns/op   B/op   allocs/op.
type Sample struct {
	Iterations  int     `json:"iterations"`
	NsPerOp     float64 `json:"nsPerOp"`
	BytesPerOp  int64   `json:"bytesPerOp"`
	AllocsPerOp int64   `json:"allocsPerOp"`
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

	for _, pkg := range pkgs {
		for _, top := range pkg.Nodes {
			collectBenchLeaves(pkg.Path, top, top, &results, index)
		}
	}

	for _, pkg := range pkgs {
		harvestPackageOutputSamples(pkg.Path, pkg.Output, &results, index)
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
		Results:       results,
	}
}

// collectBenchLeaves recurses through n's subtree, recording a Sample for
// every KindBenchmark leaf (a benchmark node with no children). top is the
// package-level ancestor node used to derive Suite; when n is itself top
// (a bare top-level benchmark with no suite wrapper), Suite is left empty.
func collectBenchLeaves(pkgPath string, top, n *gotestspec.Node, results *[]Result, index map[string]int) {
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

		sample := Sample{
			Iterations:  n.Iterations,
			NsPerOp:     n.NsPerOp,
			BytesPerOp:  n.BytesPerOp,
			AllocsPerOp: n.AllocsPerOp,
		}

		key := pkgPath + "\x00" + suite + "\x00" + n.Name
		if idx, ok := index[key]; ok {
			(*results)[idx].Samples = append((*results)[idx].Samples, sample)
			return
		}
		index[key] = len(*results)
		*results = append(*results, Result{
			Package: pkgPath,
			Suite:   suite,
			Name:    n.Name,
			Samples: []Sample{sample},
		})
		return
	}

	for _, c := range n.Children {
		collectBenchLeaves(pkgPath, top, c, results, index)
	}
}

// packageBenchLineRe mirrors gotestspec's own benchLineRe (tree.go), except
// it additionally captures the benchmark's full name (including any
// "Suite/Method" nesting and the trailing "-<GOMAXPROCS>" suffix it's
// stripped from), since package-level output isn't already scoped to a
// known Test the way tagged output is.
var packageBenchLineRe = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op(?:\s+(\d+) B/op)?(?:\s+(\d+) allocs/op)?`)

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
		fullName, iters, nsPerOp, bPerOp, allocsPerOp, ok := parsePackageBenchLine(line)
		if !ok {
			continue
		}
		suite, name := splitBenchFullName(fullName)
		key := pkgPath + "\x00" + suite + "\x00" + name
		idx, ok := index[key]
		if !ok {
			continue
		}
		(*results)[idx].Samples = append((*results)[idx].Samples, Sample{
			Iterations:  iters,
			NsPerOp:     nsPerOp,
			BytesPerOp:  bPerOp,
			AllocsPerOp: allocsPerOp,
		})
	}
}

// parsePackageBenchLine parses a go test benchmark result line the same way
// gotestspec's parseBenchOutput does, additionally returning the
// benchmark's full name (e.g. "BenchmarkFooTestSuite/BenchmarkParse-8",
// GOMAXPROCS suffix included) captured from the line itself.
func parsePackageBenchLine(line string) (fullName string, iters int, nsPerOp float64, bPerOp, allocsPerOp int64, ok bool) {
	m := packageBenchLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", 0, 0, 0, 0, false
	}
	iters, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, 0, 0, 0, false
	}
	nsPerOp, err = strconv.ParseFloat(m[3], 64)
	if err != nil {
		return "", 0, 0, 0, 0, false
	}
	if m[4] != "" {
		bPerOp, _ = strconv.ParseInt(m[4], 10, 64)
	}
	if m[5] != "" {
		allocsPerOp, _ = strconv.ParseInt(m[5], 10, 64)
	}
	return m[1], iters, nsPerOp, bPerOp, allocsPerOp, true
}

// splitBenchFullName splits a benchmark's full name (as captured from a
// result line, e.g. "BenchmarkFooTestSuite/BenchmarkParse") into the Suite
// and Name fields used to key a Result, mirroring collectBenchLeaves: Suite
// is the top-level segment with its "Benchmark" prefix stripped, and Name
// is the leaf's own (last) segment. A name with no "/" is a bare top-level
// benchmark, reported with an empty Suite.
func splitBenchFullName(full string) (suite, name string) {
	parts := strings.Split(full, "/")
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.TrimPrefix(parts[0], "Benchmark"), parts[len(parts)-1]
}

// Save writes b to path as indented JSON (0644).
func Save(path string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("gotestbench: marshal baseline: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
