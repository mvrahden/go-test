// Package mutate writes the mutants of a change's lines into one build
// (mutant schemata): every mutant of a package's changed lines is compiled
// in, each switched on by the id the binary reads once from EnvMutant. It
// also reads the comments that keep a line from being mutated, maps
// per-suite coverage to the mutants it executes and orders mutants for a
// time budget. Nothing here runs a test.
package mutate

import (
	"cmp"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// EnvMutant names the variable a schemata build reads its active mutant
// from; unset or 0 switches none on.
const EnvMutant = "GOTEST_MUTANT"

// HelperFile is the file the overlay adds to each mutated package.
const HelperFile = "gotest_mutant_schemata.go"

// Operators, in the order a line's mutants are judged.
const (
	OpNegate   = "negate-compare" // == !=, < >=, > <=
	OpBoundary = "boundary"       // < <=, > >=
	OpLogical  = "logical"        // && ||
	OpCond     = "negate-cond"    // an if or for condition negated
	OpReturn   = "early-return"   // return zero values before a changed statement
	OpDelete   = "delete-stmt"    // an expression statement or assignment removed
	OpLiteral  = "literal"        // true false, 0 1
	OpString   = "string"         // "…" "", "" "gotest-mutant"; asked for apart
)

// Operators lists the operators in judging order.
var Operators = []string{OpNegate, OpBoundary, OpLogical, OpCond, OpReturn, OpDelete, OpLiteral, OpString}

// Mutant is one change the schemata can switch on.
type Mutant struct {
	ID      int
	Package string
	// File is absolute.
	File string
	Line int
	Col  int
	// Func is the function the line lies in, "" at package level: a method
	// as Type.Method.
	Func     string
	Op       string
	Original string
	Mutated  string
}

// Plan is a schemata build's overlay content and the mutants it holds.
type Plan struct {
	Mutants []Mutant
	// Files maps an absolute source path to the source to compile in its
	// place; a path absent on disk is a file the overlay adds.
	Files map[string][]byte
	// Skipped names the files of a changed line that get no mutant at all,
	// each with why.
	Skipped []string
}

// Changed is, per absolute file path, the head lines a change touched.
type Changed map[string]map[int]bool

// Options narrow what Generate writes.
type Options struct {
	// Strings adds the string literal operator.
	Strings bool
	// Drop lists mutant ids a previous build refused or a comment ignores;
	// their sites are left as written. Ids stay those of the full set.
	Drop map[int]bool
}

// Load loads the packages of the directories holding the changed files,
// with their syntax and types, for Generate. A package that does not
// compile comes back with its errors.
func Load(dir string, changed Changed, buildFlags []string) ([]*packages.Package, error) {
	var dirs []string
	for f := range changed {
		dirs = append(dirs, "./"+filepath.ToSlash(relTo(dir, filepath.Dir(f))))
	}
	slices.Sort(dirs)
	dirs = slices.Compact(dirs)
	if len(dirs) == 0 {
		return nil, nil
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode:       packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:        dir,
		BuildFlags: buildFlags,
	}, dirs...)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(pkgs, func(a, b *packages.Package) int { return cmp.Compare(a.PkgPath, b.PkgPath) })
	return pkgs, nil
}

func relTo(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

// Generate writes the schemata of every mutable site on a changed line of
// pkgs' non-test files. Ids go from 1 in package, file and position order,
// the same for the same inputs, so Drop names the same mutants.
func Generate(pkgs []*packages.Package, changed Changed, opt Options) (*Plan, error) {
	plan := &Plan{Files: map[string][]byte{}}
	next := 1
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("%s does not compile: %v", pkg.PkgPath, pkg.Errors[0])
		}
		order := make([]int, len(pkg.Syntax))
		for i := range order {
			order[i] = i
		}
		slices.SortFunc(order, func(a, b int) int { return cmp.Compare(pkg.CompiledGoFiles[a], pkg.CompiledGoFiles[b]) })
		picks := map[string]bool{}
		for _, i := range order {
			f := pkg.Syntax[i]
			name := pkg.CompiledGoFiles[i]
			lines := changed[name]
			if len(lines) == 0 || strings.HasSuffix(name, "_test.go") {
				continue
			}
			switch {
			case importsC(f):
				plan.Skipped = append(plan.Skipped, name+": a cgo file")
				continue
			case ast.IsGenerated(f):
				plan.Skipped = append(plan.Skipped, name+": a generated file")
				continue
			}
			src, err := os.ReadFile(name) //nolint:gosec // G304: a package file the loader listed
			if err != nil {
				return nil, err
			}
			g := &gen{pkg: pkg, file: f, src: src, lines: lines, path: name, opt: opt, picks: picks}
			g.collect()
			out, muts := g.render(&next)
			plan.Mutants = append(plan.Mutants, muts...)
			if g.rendered {
				plan.Files[name] = out
			}
		}
		if len(plan.Files) > 0 && hasFileIn(plan.Files, pkg) {
			dir := filepath.Dir(pkg.CompiledGoFiles[0])
			plan.Files[filepath.Join(dir, HelperFile)] = helperFile(pkg.Name, picks)
		}
	}
	return plan, nil
}

// hasFileIn reports whether files holds a file of pkg.
func hasFileIn(files map[string][]byte, pkg *packages.Package) bool {
	for _, name := range pkg.CompiledGoFiles {
		if _, ok := files[name]; ok {
			return true
		}
	}
	return false
}

func importsC(f *ast.File) bool {
	for _, im := range f.Imports {
		if im.Path.Value == `"C"` {
			return true
		}
	}
	return false
}
