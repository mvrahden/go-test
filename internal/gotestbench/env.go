package gotestbench

import (
	"fmt"
	"strings"
)

// EnvDiff is one recorded environment field in which a baseline and the run
// compared against it disagree.
type EnvDiff struct {
	Field    string `json:"field"`    // "goos", "goarch" or "goVersion"
	Baseline string `json:"baseline"` // what the baseline recorded
	Run      string `json:"run"`      // what this run reports
}

// EnvMismatch names the environment fields in which two baselines disagree.
//
// Benchmark numbers describe one toolchain on one machine class. Compared
// across either, a delta measures the difference between the machines, not
// the change in the code — so a mismatch makes every row of the table
// suspect. It is reported and never enforced: only the operator knows
// whether two runners are alike enough to mean something, and a run that
// refused would strand anyone deliberately comparing across a toolchain
// upgrade. A field the baseline left empty is unknown, not different.
func EnvMismatch(old, new Baseline) []EnvDiff { //nolint:gocritic // hugeParam: mirrors Compare
	var diffs []EnvDiff
	for _, f := range []struct{ field, was, now string }{
		{"goos", old.GOOS, new.GOOS},
		{"goarch", old.GOARCH, new.GOARCH},
		{"goVersion", old.GoVersion, new.GoVersion},
	} {
		if f.was == "" || f.now == "" || f.was == f.now {
			continue
		}
		diffs = append(diffs, EnvDiff{Field: f.field, Baseline: f.was, Run: f.now})
	}
	return diffs
}

// FormatEnvMismatch writes the one line that names a mismatch, so the
// terminal, the step summary and the report agree on the wording.
func FormatEnvMismatch(diffs []EnvDiff) string {
	if len(diffs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(diffs))
	for _, d := range diffs {
		parts = append(parts, fmt.Sprintf("%s %s, now %s", d.Field, d.Baseline, d.Run))
	}
	return "baseline was recorded in a different environment (" + strings.Join(parts, "; ") + "); the deltas compare runs that are not alike"
}
