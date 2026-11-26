package lint

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/about"

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
	// Unapplied names the fixes Fix dropped because their result does not
	// compile; their findings stay in Findings.
	Unapplied []Finding
	// Suppressions are the gotest nolint comments of the analyzed files,
	// sorted by module, file and position.
	Suppressions []Suppression
	graph        *checker.Graph
}

// Suppression is one nolint comment that can suppress a gotest rule.
type Suppression struct {
	Module string   `json:"module"` // module path; "" outside a module
	File   string   `json:"file"`   // slash-separated, relative to the module root
	Line   int      `json:"line"`
	Col    int      `json:"col"`
	Rules  []string `json:"rules"`  // the ids named, sorted; "all" for a bare one
	Reason string   `json:"reason"` // "" when it gives none
	// Suppressed says the comment suppressed a finding in this run.
	Suppressed bool `json:"suppressed"`
}

// PrintJSON writes the findings in the go/analysis driver's -json format.
func (r *Result) PrintJSON(w io.Writer) error {
	return r.graph.PrintJSON(w)
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

	// dropped-case judges a case over both test packages of its directory.
	defer registerDirCases(pkgs)()
	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}
	noteAnalyzed(graph.Roots)
	return newResult(graph)
}

// newResult reads the findings of graph's roots.
func newResult(graph *checker.Graph) (*Result, error) {
	res := &Result{graph: graph}
	seen := map[Finding]bool{}
	seenDirective := map[token.Position]int{} // → index in res.Suppressions
	fixFiles := map[string]bool{}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, fmt.Errorf("analyze %s: %w", act.Package.PkgPath, act.Err)
		}
		res.addSuppressions(act, seenDirective)
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
	slices.SortFunc(res.Suppressions, func(a, b Suppression) int {
		return cmp.Or(
			cmp.Compare(a.Module, b.Module),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
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

// addSuppressions adds the nolint comments act's pass listed. A file both
// variants of a package hold is listed once, suppressing where either did.
func (r *Result) addSuppressions(act *checker.Action, seen map[token.Position]int) {
	pr, _ := act.Result.(*passResult)
	if pr == nil {
		return
	}
	module, root := "", ""
	if m := act.Package.Module; m != nil {
		module, root = m.Path, m.Dir
	}
	for _, d := range pr.Directives {
		pos := act.Package.Fset.Position(d.Pos)
		key := token.Position{Filename: pos.Filename, Line: pos.Line, Column: pos.Column}
		if i, ok := seen[key]; ok {
			r.Suppressions[i].Suppressed = r.Suppressions[i].Suppressed || d.Used
			continue
		}
		seen[key] = len(r.Suppressions)
		r.Suppressions = append(r.Suppressions, Suppression{
			Module:     module,
			File:       moduleRelative(root, pos.Filename),
			Line:       pos.Line,
			Col:        pos.Column,
			Rules:      d.Rules,
			Reason:     d.Reason,
			Suppressed: d.Used,
		})
	}
}

// moduleRelative is file relative to root, slash-separated; file itself
// where root does not hold it.
func moduleRelative(root, file string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, file); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(file)
}

// suppressionsDocument is what gotest lint -suppressions prints. Version
// opens it: the CLI that produced the document.
type suppressionsDocument struct {
	Version      string        `json:"version"`
	Suppressions []Suppression `json:"suppressions"`
}

// WriteSuppressionsJSON writes the run's suppressions as one document.
func (r *Result) WriteSuppressionsJSON(w io.Writer) error {
	doc := suppressionsDocument{Version: about.ResolvedVersion(), Suppressions: r.Suppressions}
	if doc.Suppressions == nil {
		doc.Suppressions = []Suppression{}
	}
	return json.NewEncoder(w).Encode(doc)
}
