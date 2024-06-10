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
	pkgName, ptestSrcs, pxtestSrcs, err := generateSrcs(targetPath)
	if err != nil {
		return "", nil, nil, err
	}
	return pkgName, ptestSrcs, pxtestSrcs, nil
}

func loadPackages(targetPkg string) (ptest *packages.Package, pxtest *packages.Package, _ error) {
	totalFoundPkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed loading packages. err: %w", err)
	}

	// filter all test-related packages
	testPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	if len(testPkgs) == 0 {
		return nil, nil, fmt.Errorf("no test files found")
	}
	if len(testPkgs) > 2 {
		return nil, nil, fmt.Errorf("loaded unexpected amount of packages. want: n <= 2, got: %d", len(testPkgs))
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
	return ptest, pxtest, nil
}

func generateSrcs(targetPkg string) (string, []byte, []byte, error) {
	ptest, pxtest, err := loadPackages(targetPkg)
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

	ptestSpec, err := c.ApplyGoTestSpecs(ptestCollected.Suites)
	if err != nil {
		return "", nil, nil, err
	}

	pxtestSpec, err := c.ApplyGoTestSpecs(pxtestCollected.Suites)
	if err != nil {
		return "", nil, nil, err
	}

	r := renderer{}
	ptestBuf, err := r.RenderGoTestSpec(ptest, ptestSpec)
	if err != nil {
		return "", nil, nil, err
	}
	pxtestBuf, err := r.RenderGoTestSpec(pxtest, pxtestSpec)
	if err != nil {
		return "", nil, nil, err
	}
	pkgPath := strings.TrimSuffix(takeNonNil(ptest, pxtest).PkgPath, "_test")
	return pkgPath, ptestBuf, pxtestBuf, nil
}

func takeNonNil(pkgs ...*packages.Package) *packages.Package {
	for _, v := range pkgs {
		if v != nil {
			return v
		}
	}
	panic("no test package found")
}
