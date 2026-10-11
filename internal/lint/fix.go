package lint

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"maps"
	"os"
	"slices"
	"strconv"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/ast/astutil"
)

// fixPasses bounds the passes of one Fix. A pass applies every fix that
// overlaps no other; the next applies what it held back or revealed.
const fixPasses = 10

// Change is one file Fix rewrote.
type Change struct {
	File   string
	Before []byte // the file on disk
	After  []byte // what -fix writes
}

// Fix applies the fixes res carries in memory, exactly as the go/analysis
// driver's -fix would write them, and re-analyzes the fixed sources until no
// fix is left or a pass changes nothing. It writes nothing: it returns the
// analysis of the fixed sources and the files that changed, sorted. Fixed
// sources that no longer load are an error, with no changes.
func Fix(ctx context.Context, dir string, patterns []string, tests bool, res *Result) (*Result, []Change, error) {
	f := &fixer{current: map[string][]byte{}, original: map[string][]byte{}}
	defer f.forget()
	for range fixPasses {
		if len(res.FixFiles) == 0 {
			break
		}
		progress, err := f.apply(res.graph)
		if err != nil {
			return nil, nil, err
		}
		if !progress {
			break
		}
		if res, err = analyze(ctx, dir, patterns, tests, f.current); err != nil {
			return nil, nil, fmt.Errorf("after applying fixes: %w", err)
		}
	}
	return res, f.changes(), nil
}

// fixer holds the fixed contents across passes.
type fixer struct {
	current  map[string][]byte // fixed contents, by file
	original map[string][]byte // disk contents of the files in current
}

// sources are the fixed contents analyzers read instead of the disk: the
// checker hands them os.ReadFile as Pass.ReadFile, which sees no overlay.
var sources sync.Map

// passSource reads file the way the analyzed syntax was parsed.
func passSource(pass *analysis.Pass, file string) ([]byte, error) {
	if src, ok := sources.Load(file); ok {
		return src.([]byte), nil
	}
	return pass.ReadFile(file)
}

func (f *fixer) forget() {
	for file := range f.current {
		sources.Delete(file)
	}
}

// changes returns the files whose fixed contents differ from the disk.
func (f *fixer) changes() []Change {
	var out []Change
	for file, after := range f.current {
		if before := f.original[file]; !bytes.Equal(before, after) {
			out = append(out, Change{File: file, Before: before, After: after})
		}
	}
	slices.SortFunc(out, func(a, b Change) int { return cmp.Compare(a.File, b.File) })
	return out
}

// edit replaces the bytes [start, end) of a file.
type edit struct {
	start, end int
	text       string
}

// apply runs one pass of the driver's -fix over graph (x/tools
// internal/analysis/driverutil.ApplyFixes): each diagnostic's first fix, in
// order, merged onto the fixes before it, skipping any that touches a
// generated file or conflicts; then each file is formatted with the imports
// the edits left unused removed. It reports whether any content changed.
func (f *fixer) apply(graph *checker.Graph) (bool, error) {
	type fixAct struct {
		fix *analysis.SuggestedFix
		act *checker.Action
	}
	generated := map[*token.File]bool{}
	var fixes []fixAct
	for _, act := range graph.Roots {
		for _, file := range act.Package.Syntax {
			tf := act.Package.Fset.File(file.FileStart)
			if _, seen := generated[tf]; !seen {
				generated[tf] = ast.IsGenerated(file)
			}
		}
		for i := range act.Diagnostics {
			if d := &act.Diagnostics[i]; len(d.SuggestedFixes) > 0 {
				fixes = append(fixes, fixAct{&d.SuggestedFixes[0], act})
			}
		}
	}

	baselines := map[string][]byte{}
	baseline := func(file string) ([]byte, error) {
		if src, ok := baselines[file]; ok {
			return src, nil
		}
		src, ok := f.current[file]
		if !ok {
			var err error
			if src, err = os.ReadFile(file); err != nil {
				return nil, err
			}
		}
		baselines[file] = src
		return src, nil
	}

	accumulated := map[string][]edit{}
	filePkgs := map[string]*types.Package{}
fixes:
	for _, fa := range fixes {
		fset := fa.act.Package.Fset
		for _, e := range fa.fix.TextEdits {
			if generated[fset.File(e.Pos)] {
				continue fixes
			}
		}
		fileEdits := map[string][]edit{}
		for _, e := range fa.fix.TextEdits {
			tf := fset.File(e.Pos)
			filePkgs[tf.Name()] = fa.act.Package.Types
			src, err := baseline(tf.Name())
			if err != nil {
				continue fixes
			}
			// A size mismatch means the file changed since it was parsed.
			if tf.Size() != len(src) {
				return false, fmt.Errorf("concurrent file modification detected in file %s (size changed from %d -> %d bytes); aborting fix",
					tf.Name(), tf.Size(), len(src))
			}
			fileEdits[tf.Name()] = append(fileEdits[tf.Name()], edit{tf.Offset(e.Pos), tf.Offset(e.End), string(e.NewText)})
		}
		merged := map[string][]edit{}
		for file, edits := range fileEdits {
			if prev := accumulated[file]; len(prev) > 0 {
				var ok bool
				if edits, ok = mergeEdits(prev, edits); !ok {
					continue fixes
				}
			}
			merged[file] = edits
		}
		maps.Copy(accumulated, merged)
	}

	progress := false
	for _, file := range slices.Sorted(maps.Keys(accumulated)) {
		edits := accumulated[file]
		if len(edits) == 0 {
			continue
		}
		src := baselines[file]
		fixed, err := applyEdits(src, edits)
		if err != nil {
			return false, fmt.Errorf("applying fixes to %s: %w", file, err)
		}
		if formatted, err := formatRemovingImports(filePkgs[file], fixed); err == nil {
			fixed = formatted
		}
		if _, ok := f.original[file]; !ok {
			f.original[file] = src
		}
		if !bytes.Equal(src, fixed) {
			progress = true
		}
		f.current[file] = fixed
		sources.Store(file, fixed)
	}
	return progress, nil
}

// mergeEdits merges two sorted edit lists as a three-way merge would:
// identical edits coalesce, and edits that overlap otherwise conflict.
func mergeEdits(x, y []edit) ([]edit, bool) {
	x, y = slices.Clone(x), slices.Clone(y)
	var merged []edit
	var xi, yi int
	for xi < len(x) && yi < len(y) {
		px, py := &x[xi], &y[yi]
		switch {
		case *px == *py:
			merged = append(merged, *px)
			xi++
			yi++
		case px.end <= py.start: // x before y, or an insertion at its start
			merged = append(merged, *px)
			xi++
		case py.end <= px.start:
			merged = append(merged, *py)
			yi++
		case px.start < py.start: // split off the part of x before y
			merged = append(merged, edit{px.start, py.start, ""})
			px.start = py.start
		case py.start < px.start:
			merged = append(merged, edit{py.start, px.start, ""})
			py.start = px.start
		default: // different edits of the same bytes
			return nil, false
		}
	}
	merged = append(merged, x[xi:]...)
	return append(merged, y[yi:]...), true
}

// applyEdits applies edits, stably sorted by position, to src.
func applyEdits(src []byte, edits []edit) ([]byte, error) {
	edits = slices.Clone(edits)
	slices.SortStableFunc(edits, func(a, b edit) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.end, b.end))
	})
	var out []byte
	last := 0
	for _, e := range edits {
		if e.start < last || e.start > e.end || e.end > len(src) {
			return nil, fmt.Errorf("invalid edit [%d,%d)", e.start, e.end)
		}
		out = append(out, src[last:e.start]...)
		out = append(out, e.text...)
		last = e.end
	}
	return append(out, src[last:]...), nil
}

// formatRemovingImports formats src as gofmt does, first deleting the
// imports nothing in it references any more. It never adds an import.
func formatRemovingImports(pkg *types.Package, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	// Object resolution stays on: file.Unresolved is what references imports.
	file, err := parser.ParseFile(fset, "fixed.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	ast.SortImports(fset, file)
	removeUnusedImports(fset, pkg, file)

	// go/format's printer settings; 1<<30 is its unexported number
	// normalization mode.
	cfg := &printer.Config{Mode: printer.UseSpaces | printer.TabIndent | 1<<30, Tabwidth: 8}
	var buf bytes.Buffer
	if err := cfg.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// removeUnusedImports deletes the imports whose name nothing in file
// references. An import's name comes from pkg's dependency graph, so an
// import pkg does not know yet, a dot and a blank import all stay.
func removeUnusedImports(fset *token.FileSet, pkg *types.Package, file *ast.File) {
	names := map[string]string{} // import path → package name
	var visit func(*types.Package)
	visit = func(p *types.Package) {
		if names[p.Path()] == "" {
			names[p.Path()] = p.Name()
			for _, imp := range p.Imports() {
				visit(imp)
			}
		}
	}
	if pkg != nil {
		for _, imp := range pkg.Imports() {
			visit(imp)
		}
	}

	// The parser leaves a reference unresolved when no declaration in the
	// file binds it: a package name, among others.
	used := map[string]bool{}
	for _, id := range file.Unresolved {
		used[id.Name] = true
	}

	var unused []func()
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		explicit := ""
		if spec.Name != nil {
			explicit = spec.Name.Name
		}
		name := cmp.Or(explicit, names[path])
		switch name {
		case "", ".", "_":
			continue
		}
		if !used[name] {
			unused = append(unused, func() { astutil.DeleteNamedImport(fset, file, explicit, path) })
		}
	}
	for _, del := range unused {
		del()
	}
}
