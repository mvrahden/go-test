package main

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// vocabOf maps a declaration to the vocabulary it renders in. The two enums are
// deliberately separate — one describes source, the other display — but this is
// the single place they meet.
func vocabOf(k gotestast.BehaviorKind) gotestspec.Vocab {
	switch k {
	case gotestast.BehaviorWhen:
		return gotestspec.VocabWhen
	case gotestast.BehaviorIt:
		return gotestspec.VocabIt
	case gotestast.BehaviorEach:
		return gotestspec.VocabEach
	default:
		return gotestspec.VocabNone
	}
}

// renderedBehaviorLabel is what a human should see for a declared behavior: the
// text the developer wrote, spoken in its vocabulary. Discovery and spec
// rendering must produce it from the same input by the same rule, or the same
// behavior reads two different ways in the same editor.
func renderedBehaviorLabel(b *gotestast.Behavior) string {
	return vocabOf(b.Kind).Apply(b.Display)
}

// declarationsForStream reads declarations for the packages a captured stream
// names, from whatever source the working directory can reach. A replay is how
// the editor's Spec View renders every run, so it must read the labels the
// live run read, or the two panes spell one behavior two ways. Packages that
// cannot be loaded here — a stream from another module, a checkout without
// the source — simply contribute nothing, and their labels are reconstructed
// from the names as before. Nothing about loading may fail the render.
// Packages in have are left out: the caller read them already.
func declarationsForStream(events []gotestspec.TestEvent, have map[string]bool) gotestspec.DeclarationIndex {
	idx, _ := streamSource(events, have)
	return idx
}

// streamSource is declarationsForStream with the skip calls of the same
// source.
func streamSource(events []gotestspec.TestEvent, have map[string]bool) (gotestspec.DeclarationIndex, gotestspec.SkipCalls) {
	seen := map[string]bool{}
	var pkgs []string
	for i := range events {
		pkg := events[i].Package
		if pkg == "" || seen[pkg] || have[pkg] || events[i].RunLevel {
			continue
		}
		seen[pkg] = true
		pkgs = append(pkgs, pkg)
	}
	if len(pkgs) == 0 {
		return nil, nil
	}
	loaded, _, err := gotestgen.LoadPackages(pkgs, nil)
	if err != nil {
		return nil, nil
	}
	return buildDeclarationIndex(loaded), skipCallsOf(loaded)
}

// skipCallsOf lists, per package, the lines of its test files that call a
// Skip or Skipf method: the lines a skip's message is reported at.
func skipCallsOf(loadResults []*gotestgen.LoadResult) gotestspec.SkipCalls {
	calls := gotestspec.SkipCalls{}
	for _, lr := range loadResults {
		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			for _, f := range pkg.Syntax {
				name := pkg.Fset.Position(f.Pos()).Filename
				if !strings.HasSuffix(name, "_test.go") {
					continue
				}
				base := filepath.Base(name)
				ast.Inspect(f, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || (sel.Sel.Name != "Skip" && sel.Sel.Name != "Skipf") {
						return true
					}
					if calls[lr.PkgPath] == nil {
						calls[lr.PkgPath] = map[string]bool{}
					}
					// A call over several lines may be reported at any of them.
					for line := pkg.Fset.Position(call.Pos()).Line; line <= pkg.Fset.Position(call.End()).Line; line++ {
						calls[lr.PkgPath][fmt.Sprintf("%s:%d", base, line)] = true
					}
					return true
				})
			}
		}
	}
	return calls
}

// buildDeclarationIndex reads what every declared behavior is called, keyed by
// the test path go test will print for it. A run's event stream says which
// subtests happened, but not what the developer called them: by the time a
// description has become a subtest name, every space in it is an underscore and
// the ones that were already underscores are indistinguishable.
//
// Behaviors that source cannot enumerate — a When inside a condition, a table
// that is not a literal — are simply absent, and their labels are reconstructed
// from their names as before. That is the same floor `behaviorsComplete`
// reports, not a separate failure.
func buildDeclarationIndex(loadResults []*gotestgen.LoadResult) gotestspec.DeclarationIndex {
	idx := gotestspec.DeclarationIndex{}
	collector := gotestgen.NewCollector()

	for _, lr := range loadResults {
		// A suite or a stdlib test both test packages declare is a twin
		// pair: its second instance in a run is the external one, not a
		// repeat.
		declaredIn := map[string]int{}
		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			names := testFuncNames(pkg)
			result := collector.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				for name := range names {
					declaredIn[name]++
				}
				continue
			}
			for _, suite := range result.Suites {
				names["Test"+suite.Identifier()] = true
			}
			for name := range names {
				declaredIn[name]++
			}
			for _, suite := range result.Suites {
				suitePath := "Test" + suite.Identifier()
				for _, method := range suite.TestCases() {
					indexBehaviors(idx, lr.PkgPath, []string{suitePath, method.Identifier()}, suite.BehaviorsOf(method).Behaviors)
				}
			}
		}
		for suitePath, n := range declaredIn {
			if n < 2 {
				continue
			}
			if idx[lr.PkgPath] == nil {
				idx[lr.PkgPath] = map[string]gotestspec.Declaration{}
			}
			idx[lr.PkgPath][gotestspec.DeclKey(suitePath)] = gotestspec.Declaration{Twin: true}
		}
	}

	if len(idx) == 0 {
		return nil
	}
	return idx
}

// testFuncNames are the top-level Test functions pkg's test files declare.
func testFuncNames(pkg *packages.Package) map[string]bool {
	names := map[string]bool{}
	for _, f := range pkg.Syntax {
		if !strings.HasSuffix(pkg.Fset.Position(f.Pos()).Filename, "_test.go") {
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" || !isTestName(fn.Name.Name) {
				continue
			}
			names[fn.Name.Name] = true
		}
	}
	return names
}

// isTestName is go test's rule: Test, then nothing or a character that is
// not a lower-case letter.
func isTestName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Test")
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

func indexBehaviors(idx gotestspec.DeclarationIndex, pkgPath string, parent []string, behaviors []*gotestast.Behavior) {
	for _, b := range behaviors {
		// The level is the name as written, without a repeat suffix.
		path := append(append([]string(nil), parent...), b.Written())
		paths := idx[pkgPath]
		if paths == nil {
			paths = map[string]gotestspec.Declaration{}
			idx[pkgPath] = paths
		}
		paths[gotestspec.DeclKey(path...)] = gotestspec.Declaration{Label: b.Display, Vocab: vocabOf(b.Kind)}
		indexBehaviors(idx, pkgPath, path, b.Children)
	}
}
