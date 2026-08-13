package lint

import (
	"cmp"
	"fmt"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// Finding is one lint diagnostic with a resolved position, for drivers that
// need findings as data (GitHub annotations) instead of text on stderr.
type Finding struct {
	File    string // as resolved by the loader; typically absolute
	Line    int
	Col     int
	Rule    string // the diagnostic category, i.e. the rule ID
	Message string
}

// Run executes the analyzer over the packages matched by patterns, honoring
// the flag-bound configuration (skip-*, disable-nolint) already applied to
// Analyzer.Flags. dir is the module directory to load from; "" means the
// current directory, matching the analysis driver's own resolution.
//
// Findings are deduplicated across test-variant package loads and sorted by
// position. A load or analysis failure is an error, never an empty result —
// the same success-is-proven contract PreflightLoad applies.
func Run(dir string, patterns []string) ([]Finding, error) {
	cfg := &packages.Config{
		Dir:   dir,
		Mode:  packages.LoadAllSyntax,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("cannot lint uncompilable package %s: %v", p.PkgPath, p.Errors[0])
		}
	}

	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}

	var findings []Finding
	seen := map[Finding]bool{}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("analyze %s: %w", act.Package.PkgPath, act.Err)
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			f := Finding{
				File:    pos.Filename,
				Line:    pos.Line,
				Col:     pos.Column,
				Rule:    d.Category,
				Message: d.Message,
			}
			if seen[f] {
				continue
			}
			seen[f] = true
			findings = append(findings, f)
		}
	}

	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return findings, nil
}
