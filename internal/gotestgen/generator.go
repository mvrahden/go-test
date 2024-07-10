package gotestgen

import (
	"strings"

	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

type GenerateResults []*GenerateResult
type GenerateResult struct {
	AbsPath string // Abs Package Path
	Package string // Package name
	PTest   []byte // Test Suite PTest
	PXTest  []byte // Test Suite PXTest
}

const (
	packageEvalMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo
)

func Generate(targetPath string) (GenerateResults, error) {
	res, err := generateSrcs(targetPath)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type loadResult struct {
	pkgDir  string
	pkgPath string
	ptest   *packages.Package
	pxtest  *packages.Package
}

func loadPackages(targetPkg string) ([]*loadResult, error) {
	totalFoundPkgs, err := LoadCached(targetPkg)
	if err != nil {
		return nil, err
	}

	// filter all packages with Go-Module support
	loadedTestPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
		return item.Module != nil
	})
	// filter all test-related packages
	loadedTestPkgs = slices.Filter(loadedTestPkgs, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	testPkgs := slices.ReduceSeed(loadedTestPkgs, map[string]*loadResult{}, func(p *packages.Package, acc map[string]*loadResult) map[string]*loadResult {
		isPxTest := strings.HasSuffix(p.Name, "_test")
		pkgPath := p.PkgPath
		if isPxTest {
			pkgPath = strings.TrimSuffix(pkgPath, "_test")
		}
		_, ok := acc[pkgPath]
		if !ok {
			acc[pkgPath] = &loadResult{pkgPath: pkgPath, pkgDir: DeterminePkgDir(p)}
		}
		if !isPxTest {
			acc[pkgPath].ptest = p
		} else {
			acc[pkgPath].pxtest = p
		}
		return acc
	})
	var res []*loadResult
	for _, v := range testPkgs {
		res = append(res, v)
	}
	return res, nil
}

func generateSrcs(targetPkg string) (GenerateResults, error) {
	loadResults, err := loadPackages(targetPkg)
	if err != nil {
		return nil, err
	}

	return slices.MapErr(loadResults, func(lr *loadResult, _ int) (*GenerateResult, error) {
		c := collector{}
		ptestCollected := c.CollectSuiteSpecs(lr.ptest)
		if len(ptestCollected.Errs) > 0 {
			return nil, ptestCollected.Errs[0].Err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.pxtest)
		if len(pxtestCollected.Errs) > 0 {
			return nil, ptestCollected.Errs[0].Err
		}

		ptestSpec, err := c.ApplyTestSuiteSpecs(ptestCollected.Suites)
		if err != nil {
			return nil, err
		}

		pxtestSpec, err := c.ApplyTestSuiteSpecs(pxtestCollected.Suites)
		if err != nil {
			return nil, err
		}

		r := renderer{}
		ptestBuf, err := r.RenderTestSuiteSpec(lr.ptest, ptestSpec)
		if err != nil {
			return nil, err
		}
		pxtestBuf, err := r.RenderTestSuiteSpec(lr.pxtest, pxtestSpec)
		if err != nil {
			return nil, err
		}
		return &GenerateResult{AbsPath: lr.pkgDir, Package: lr.pkgPath, PTest: ptestBuf, PXTest: pxtestBuf}, nil
	})
}

func getPkgPathOrDefault(ptest, pxtest *packages.Package, dflt string) string {
	for _, pkg := range []*packages.Package{ptest, pxtest} {
		if pkg != nil {
			return pkg.PkgPath
		}
	}
	return dflt
}
