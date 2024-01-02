package gotestgen

import (
	"go/ast"

	"github.com/mvrahden/go-test/internal/gotestast"
	goinspect "golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"
)

type collector struct{}

func (collector) CollectSuiteSpecs(pkgs []*packages.Package) ([]*gotestast.TestSuiteSpec, error) {
	var suites []*gotestast.TestSuiteSpec
	for _, pkg := range pkgs {
		insp := goinspect.New(pkg.Syntax)

		var errs []error
		// find suites
		insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
			s, _, err := gotestast.DetermineTestSuite(n, pkg)
			if err != nil {
				errs = append(errs, err)
				return
			}
			if s == nil {
				return
			}
			suites = append(suites, s)
		})
		if len(errs) > 0 {
			return nil, errs[0]
		}

		// find methods
		for _, s := range suites {
			insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
				_, err := gotestast.DetermineTestHarness(n, pkg, s)
				if err != nil {
					errs = append(errs, err)
				}
			})
		}
		if len(errs) > 0 {
			return nil, errs[0]
		}
	}
	return suites, nil
}
