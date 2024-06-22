package gotestgen

import (
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

const (
	packageEvalMode = packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo
)

func Generate(targetPath string) (string, []byte, []byte, error) {
	pkgPath, ptestSrcs, pxtestSrcs, err := generateSrcs(targetPath)
	if err != nil {
		return "", nil, nil, err
	}
	return pkgPath, ptestSrcs, pxtestSrcs, nil
}

func loadPackages(targetPkg string) (pkgPath string, ptest *packages.Package, pxtest *packages.Package, _ error) {
	totalFoundPkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed loading packages. err: %w", err)
	}
	if len(totalFoundPkgs) == 0 || totalFoundPkgs[0].Name == "" {
		return "", nil, nil, fmt.Errorf("not a Go package") // nothing found
	}
	pkgPath = strings.TrimSuffix(totalFoundPkgs[0].PkgPath, "_test")

	// filter all test-related packages
	testPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	if len(testPkgs) == 0 {
		return pkgPath, nil, nil, nil // no test files found
	}
	if len(testPkgs) > 2 {
		return "", nil, nil, fmt.Errorf("loaded unexpected amount of packages. want: n <= 2, got: %d", len(testPkgs))
	}
	_ptest, _pxtest := slices.SplitBy(testPkgs, func(p *packages.Package, _ int) bool {
		return !strings.HasSuffix(p.Name, "_test")
	})
	if len(_ptest) == 1 {
		ptest = _ptest[0]
	}
	if len(_pxtest) == 1 {
		pxtest = _pxtest[0]
	}
	return pkgPath, ptest, pxtest, nil
}

func generateSrcs(targetPkg string) (string, []byte, []byte, error) {
	pkgPath, ptest, pxtest, err := loadPackages(targetPkg)
	if err != nil {
		return "", nil, nil, err
	}
	c := collector{}
	ptestCollected := c.CollectSuiteSpecs(ptest)
	if len(ptestCollected.Errs) > 0 {
		return "", nil, nil, ptestCollected.Errs[0].Err
	}
	pxtestCollected := c.CollectSuiteSpecs(pxtest)
	if len(pxtestCollected.Errs) > 0 {
		return "", nil, nil, ptestCollected.Errs[0].Err
	}

	ptestSpec, err := c.ApplyTestSuiteSpecs(ptestCollected.Suites)
	if err != nil {
		return "", nil, nil, err
	}

	pxtestSpec, err := c.ApplyTestSuiteSpecs(pxtestCollected.Suites)
	if err != nil {
		return "", nil, nil, err
	}

	r := renderer{}
	ptestBuf, err := r.RenderTestSuiteSpec(ptest, ptestSpec)
	if err != nil {
		return "", nil, nil, err
	}
	pxtestBuf, err := r.RenderTestSuiteSpec(pxtest, pxtestSpec)
	if err != nil {
		return "", nil, nil, err
	}
	return pkgPath, ptestBuf, pxtestBuf, nil
}

func getPkgPathOrDefault(ptest, pxtest *packages.Package, dflt string) string {
	for _, pkg := range []*packages.Package{ptest, pxtest} {
		if pkg != nil {
			return pkg.PkgPath
		}
	}
	return dflt
}
