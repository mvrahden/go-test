package gotestbench

import (
	"encoding/json"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
)

// reportSchemaVersion is the version stamped on every Report this package
// writes. It is independent of the Baseline schemaVersion: the report is the
// machine-readable output of one `gotest bench --json` run, the baseline is
// the persisted comparison anchor.
const reportSchemaVersion = 1

// Report is the versioned JSON document `gotest bench --json` emits: the
// run's results in baseline shape, plus — when a comparison ran — its deltas
// and the gate verdict. Consumers (the VS Code extension) read this instead
// of scraping text, so every field is contract: additions are fine, renames
// and removals need a schema bump.
type Report struct {
	SchemaVersion int      `json:"schemaVersion"`
	Baseline      Baseline `json:"baseline"`
	Deltas        []Delta  `json:"deltas,omitempty"`
	// Missing names the baseline's benchmarks the run did not produce and
	// Added the run's benchmarks the baseline lacks: what the deltas leave out.
	Missing []string `json:"missing,omitempty"`
	Added   []string `json:"added,omitempty"`
	Gate    *Gate    `json:"gate,omitempty"`
	// EnvMismatch is non-empty when the baseline compared against was
	// recorded under a different toolchain or platform, which makes every
	// delta suspect. Present as data, not as a verdict: nothing refuses the
	// comparison, and consumers decide how loudly to say so.
	EnvMismatch []EnvDiff `json:"envMismatch,omitempty"`
	// Failures are the benchmarks, suites and packages that failed, with the
	// text they failed with: a failed benchmark has no samples to show.
	Failures []Failure `json:"failures,omitempty"`
}

// Failure is one failure of the run, keyed as Result is: a benchmark by its
// suite and name, a suite's own failure by its suite alone, a package's by
// its package alone. Output is what it printed, go test's framing lines
// left out.
type Failure struct {
	Package string `json:"package"`
	Suite   string `json:"suite,omitempty"`
	Name    string `json:"name,omitempty"`
	Output  string `json:"output"`
}

// FailuresOf lists the failures in pkgs, keyed as FromPackages keys results.
// The lines go test prints with a benchmark's name are left out, and with
// them a suite's failure that says nothing but carries its benchmark's.
func FailuresOf(pkgs []*gotestspec.Package) []Failure {
	var out []Failure
	for _, f := range gotestspec.Failures(pkgs) {
		var lines []string
		for _, line := range f.Lines {
			if !isBenchNameLine(f.Path, line) {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 && !f.Origin {
			continue
		}
		failure := Failure{Package: f.Package, Output: strings.Join(lines, "\n")}
		switch {
		case len(f.Path) == 0:
		case len(f.Path) == 1 && !isSuiteWrapper(pkgs, f.Package, f.Path[0]):
			failure.Name = f.Path[0]
		default:
			failure.Suite = strings.TrimPrefix(f.Path[0], "Benchmark")
			failure.Name = strings.Join(f.Path[1:], "/")
		}
		out = append(out, failure)
	}
	return out
}

// isBenchNameLine reports whether line is the name go test prints for the
// benchmark at path or one above it.
func isBenchNameLine(path []string, line string) bool {
	line = strings.TrimSpace(line)
	for i := range path {
		if line == strings.Join(path[:i+1], "/") {
			return true
		}
	}
	return false
}

// isSuiteWrapper reports whether the top node name in package path holds
// benchmarks of its own: a suite, not a bare benchmark.
func isSuiteWrapper(pkgs []*gotestspec.Package, path, name string) bool {
	for _, pkg := range pkgs {
		if pkg.Path != path {
			continue
		}
		for _, n := range pkg.Nodes {
			if n.Name == name {
				return len(n.Children) > 0
			}
		}
	}
	return false
}

// Gate is the report's gate verdict: the configured threshold, the worst
// significant regression the comparison found, and whether it breached.
// WorstKey is empty when no significant regression exists. BreachedKeys
// lists every delta the gate rule condemns (significant AND above the
// threshold) so consumers can mark each offender without re-deriving the
// rule — the rule lives here, nowhere else.
type Gate struct {
	ThresholdPct float64  `json:"thresholdPct"`
	WorstPct     float64  `json:"worstPct"`
	WorstKey     string   `json:"worstKey,omitempty"`
	Breached     bool     `json:"breached"`
	BreachedKeys []string `json:"breachedKeys,omitempty"`
}

// NewReport assembles the document for one run. deltas may be nil when no
// comparison ran; gate may be nil when no gate is active; envMismatch is nil
// unless the baseline came from another toolchain or platform.
func NewReport(b Baseline, deltas []Delta, gate *Gate, envMismatch []EnvDiff) Report { //nolint:gocritic // hugeParam: stable API
	return Report{
		SchemaVersion: reportSchemaVersion,
		Baseline:      b,
		Deltas:        deltas,
		Gate:          gate,
		EnvMismatch:   envMismatch,
	}
}

// GateVerdict evaluates deltas against thresholdPct: the worst significant
// positive PercentChange, the Key it belongs to, and whether it exceeds the
// threshold. This is the single source for both the report's gate object and
// the CLI's failure message.
func GateVerdict(deltas []Delta, thresholdPct float64) Gate {
	g := Gate{ThresholdPct: thresholdPct}
	for _, d := range deltas {
		if !d.Significant {
			continue
		}
		if d.PercentChange > g.WorstPct {
			g.WorstPct = d.PercentChange
			g.WorstKey = d.Key
		}
		if d.PercentChange > thresholdPct {
			g.BreachedKeys = append(g.BreachedKeys, d.Key)
		}
	}
	g.Breached = g.WorstPct > thresholdPct
	return g
}

// MarshalReport renders r as indented JSON, the exact bytes `--json` writes
// to stdout.
func MarshalReport(r Report) ([]byte, error) { //nolint:gocritic // hugeParam: stable API
	return json.MarshalIndent(r, "", "  ")
}
