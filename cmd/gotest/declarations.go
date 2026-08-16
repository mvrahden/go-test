package main

import (
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

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
		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			result := collector.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				continue
			}
			for _, suite := range result.Suites {
				suitePath := "Test" + suite.Identifier()
				for _, method := range suite.TestCases() {
					methodPath := suitePath + "/" + method.Identifier()
					indexBehaviors(idx, lr.PkgPath, methodPath, suite.BehaviorsOf(method).Behaviors)
				}
			}
		}
	}

	if len(idx) == 0 {
		return nil
	}
	return idx
}

func indexBehaviors(idx gotestspec.DeclarationIndex, pkgPath, parentPath string, behaviors []*gotestast.Behavior) {
	for _, b := range behaviors {
		// A declared behavior carries the exact go test segment, "#01" and all.
		// The tree keys duplicates under the name the developer wrote, so the
		// index drops the suffix the same way — otherwise the second of two
		// same-named siblings would find no declaration and read differently
		// from the first.
		path := parentPath + "/" + gotestspec.StripDuplicateSuffix(b.Name)
		paths := idx[pkgPath]
		if paths == nil {
			paths = map[string]gotestspec.Declaration{}
			idx[pkgPath] = paths
		}
		paths[path] = gotestspec.Declaration{Label: b.Display}
		indexBehaviors(idx, pkgPath, path, b.Children)
	}
}
