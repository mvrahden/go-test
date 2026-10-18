package lint

import (
	"context"

	"golang.org/x/tools/go/packages"
)

var ExportParseNolint = parseNolint

func ExportSetDisableNolint(v bool) { cfg.disableNolint = v }

func ExportSetSkip(r Rule, v bool) { *cfg.skip[r] = v }

// ExportLoad loads the targets from disk.
func ExportLoad(dir string, patterns []string, tests bool) ([]*packages.Package, error) {
	return load(context.Background(), &Targets{Dir: dir, Patterns: patterns, Tests: tests}, nil)
}

var (
	ExportIsBenchmarkMethodName = isBenchmarkMethodName
	ExportIsFuzzMethodName      = isFuzzMethodName
)
