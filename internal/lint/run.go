package lint

import (
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"slices"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

// Finding is one lint diagnostic with a resolved position, for drivers that
// need findings as data (GitHub annotations) instead of text on stderr.
type Finding struct {
	File    string // as resolved by the loader; typically absolute
	Line    int
	Col     int
	EndLine int
	Rule    string // the diagnostic category, i.e. the rule ID
	Message string
	Fixable bool // carries a fix -fix applies
}

// Result is one analysis of the lint targets.
type Result struct {
	Findings []Finding // deduplicated, sorted by position
	// FixFiles are the files the fixable findings edit.
	FixFiles []string
	graph    *checker.Graph
}

// PrintJSON writes the findings in the go/analysis driver's -json format.
func (r *Result) PrintJSON(w io.Writer) error {
	return r.graph.PrintJSON(w)
}

// Run executes the analyzer over the packages matched by patterns, test
// files included. See Analyze.
func Run(dir string, patterns []string) ([]Finding, error) {
	res, err := Analyze(context.Background(), &Targets{Dir: dir, Patterns: patterns, Tests: true})
	if err != nil {
		return nil, err
	}
	return res.Findings, nil
}

// Targets are the packages a run lints and how they load.
type Targets struct {
	// Dir is the module directory to load from; "" means the current
	// directory, matching the analysis driver's own resolution.
	Dir      string
	Patterns []string
	Tests    bool // include test files, like the driver's -test flag
	// BuildFlags go to go list, the loads of the fixed sources included:
	// -tags=integration, say.
	BuildFlags []string
	// TempDir holds the load's temporary files, the go command's work
	// directories included; "" means the system's. Removing it after a
	// stopped load removes what the go command left there.
	TempDir string
}

// Analyze executes the analyzer over the targets, honoring the flag-bound
// configuration (skip-*, disable-nolint) already applied to Analyzer.Flags.
// Canceling ctx stops the load.
//
// Findings are deduplicated across test-variant package loads and sorted by
// position. A load or analysis failure is an error, never an empty result:
// nothing was proven about a package that was not analyzed.
func Analyze(ctx context.Context, t *Targets) (*Result, error) {
	return analyze(ctx, t, nil)
}

// analyze is Analyze with sources read from overlay where it has them.
func analyze(ctx context.Context, t *Targets, overlay map[string][]byte) (*Result, error) {
	pkgs, err := load(ctx, t, overlay)
	if err != nil {
		return nil, err
	}

	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}

	res := &Result{graph: graph}
	seen := map[Finding]bool{}
	fixFiles := map[string]bool{}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("analyze %s: %w", act.Package.PkgPath, act.Err)
		}
		generated := generatedFiles(act.Package.Fset, act.Package.Syntax)
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			end := act.Package.Fset.Position(d.End)
			if !end.IsValid() {
				end = pos
			}
			edited := fixEdits(act.Package.Fset, &d, generated)
			f := Finding{
				File:    pos.Filename,
				Line:    pos.Line,
				Col:     pos.Column,
				EndLine: end.Line,
				Rule:    d.Category,
				Message: d.Message,
				Fixable: len(edited) > 0,
			}
			for _, file := range edited {
				fixFiles[file] = true
			}
			if seen[f] {
				continue
			}
			seen[f] = true
			res.Findings = append(res.Findings, f)
		}
	}

	slices.SortFunc(res.Findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.Message, b.Message),
		)
	})
	for file := range fixFiles {
		res.FixFiles = append(res.FixFiles, file)
	}
	slices.Sort(res.FixFiles)
	return res, nil
}

// generatedFiles names the generated files among syntax.
func generatedFiles(fset *token.FileSet, syntax []*ast.File) map[string]bool {
	generated := map[string]bool{}
	for _, f := range syntax {
		if ast.IsGenerated(f) {
			generated[fset.File(f.FileStart).Name()] = true
		}
	}
	return generated
}

// fixEdits returns the files the driver's -fix would edit for d: those of
// its first fix, or none when that fix touches a generated file, which the
// driver skips.
func fixEdits(fset *token.FileSet, d *analysis.Diagnostic, generated map[string]bool) []string {
	if len(d.SuggestedFixes) == 0 {
		return nil
	}
	var files []string
	for _, e := range d.SuggestedFixes[0].TextEdits {
		name := fset.File(e.Pos).Name()
		if generated[name] {
			return nil
		}
		files = append(files, name)
	}
	return files
}
