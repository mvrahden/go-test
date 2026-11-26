package lint

import (
	"context"

	"golang.org/x/tools/go/packages"
)

var ExportParseNolint = parseNolint

// ExportNolintReason is the reason a nolint comment gives.
func ExportNolintReason(text string) string {
	_, reason, _ := nolintIDs(text)
	return reason
}

func ExportSetDisableNolint(v bool) { cfg.disableNolint = v }

func ExportSetSkip(r Rule, v bool) { *cfg.skip[r] = v }

func ExportSetNolintAllow(v string) { cfg.nolintAllow = v }

// Run executes the analyzer over the packages matched by patterns, test
// files included. See Analyze.
func Run(dir string, patterns []string) ([]Finding, error) {
	res, err := Analyze(context.Background(), &Targets{Dir: dir, Patterns: patterns, Tests: true})
	if err != nil {
		return nil, err
	}
	return res.Findings, nil
}

// ExportLoad loads the targets from disk.
func ExportLoad(dir string, patterns []string, tests bool) ([]*packages.Package, error) {
	return load(context.Background(), &Targets{Dir: dir, Patterns: patterns, Tests: tests}, nil)
}

var (
	ExportIsBenchmarkMethodName = isBenchmarkMethodName
	ExportIsFuzzMethodName      = isFuzzMethodName
)

// ExportOnAnalyze makes f see the package paths of every analysis until
// the returned restore runs.
func ExportOnAnalyze(f func(paths []string)) (restore func()) {
	analyzed = f
	return func() { analyzed = nil }
}
