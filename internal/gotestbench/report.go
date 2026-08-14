package gotestbench

import "encoding/json"

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
	Gate          *Gate    `json:"gate,omitempty"`
}

// Gate is the report's gate verdict: the configured threshold, the worst
// significant regression the comparison found, and whether it breached.
// WorstKey is empty when no significant regression exists.
type Gate struct {
	ThresholdPct float64 `json:"thresholdPct"`
	WorstPct     float64 `json:"worstPct"`
	WorstKey     string  `json:"worstKey,omitempty"`
	Breached     bool    `json:"breached"`
}

// NewReport assembles the document for one run. deltas may be nil when no
// comparison ran; gate may be nil when no gate is active.
func NewReport(b Baseline, deltas []Delta, gate *Gate) Report {
	return Report{
		SchemaVersion: reportSchemaVersion,
		Baseline:      b,
		Deltas:        deltas,
		Gate:          gate,
	}
}

// GateVerdict evaluates deltas against thresholdPct: the worst significant
// positive PercentChange, the Key it belongs to, and whether it exceeds the
// threshold. This is the single source for both the report's gate object and
// the CLI's failure message.
func GateVerdict(deltas []Delta, thresholdPct float64) Gate {
	g := Gate{ThresholdPct: thresholdPct}
	for _, d := range deltas {
		if d.Significant && d.PercentChange > g.WorstPct {
			g.WorstPct = d.PercentChange
			g.WorstKey = d.Key
		}
	}
	g.Breached = g.WorstPct > thresholdPct
	return g
}

// MarshalReport renders r as indented JSON, the exact bytes `--json` writes
// to stdout.
func MarshalReport(r Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
