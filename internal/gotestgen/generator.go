package gotestgen

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
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
	packageEvalMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
)

func Generate(targetPath string) (GenerateResults, error) {
	res, _, err := generateSrcs(targetPath)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GenerateWithCollectorResults generates test suite sources and also returns
// the raw collector results, which can be used to discover shared fixtures.
func GenerateWithCollectorResults(targetPath string) (GenerateResults, []CollectorResult, error) {
	return generateSrcs(targetPath)
}

func Collect(targetPath string) (gotestast.TestSuiteSpecSet, error) {
	loadResults, err := LoadPackages(targetPath)
	if err != nil {
		return nil, err
	}
	var allSuites gotestast.TestSuiteSpecSet
	c := collector{}
	for _, lr := range loadResults {
		ptestCollected := c.CollectSuiteSpecs(lr.Ptest)
		if len(ptestCollected.Errs) > 0 {
			return nil, ptestCollected.Errs[0].Err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.Pxtest)
		if len(pxtestCollected.Errs) > 0 {
			return nil, pxtestCollected.Errs[0].Err
		}
		allSuites = append(allSuites, ptestCollected.Suites...)
		allSuites = append(allSuites, pxtestCollected.Suites...)
	}
	return allSuites, nil
}

// LoadResult holds the parsed packages for a given import path,
// split into internal-test (ptest) and external-test (pxtest) packages.
type LoadResult struct {
	PkgDir  string
	PkgPath string
	Ptest   *packages.Package
	Pxtest  *packages.Package
}

// LoadPackages loads and groups test packages for the given target pattern.
func LoadPackages(targetPkg string) ([]*LoadResult, error) {
	totalFoundPkgs, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, err
	}

	// filter all packages with Go-Module support
	loadedTestPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
		return item.Module != nil
	})
	if len(loadedTestPkgs) == 0 {
		return nil, nil
	}
	// filter all test-related packages
	loadedTestPkgs = slices.Filter(loadedTestPkgs, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	testPkgs := slices.ReduceSeed(loadedTestPkgs, map[string]*LoadResult{}, func(p *packages.Package, acc map[string]*LoadResult) map[string]*LoadResult {
		isPxTest := strings.HasSuffix(p.Name, "_test")
		pkgPath := p.PkgPath
		if isPxTest {
			pkgPath = strings.TrimSuffix(pkgPath, "_test")
		}
		_, ok := acc[pkgPath]
		if !ok {
			acc[pkgPath] = &LoadResult{PkgPath: pkgPath, PkgDir: DeterminePkgDir(p)}
		}
		if !isPxTest {
			acc[pkgPath].Ptest = p
		} else {
			acc[pkgPath].Pxtest = p
		}
		return acc
	})
	var res []*LoadResult
	for _, v := range testPkgs {
		res = append(res, v)
	}
	return res, nil
}

func generateSrcs(targetPkg string) (GenerateResults, []CollectorResult, error) {
	loadResults, err := LoadPackages(targetPkg)
	if err != nil {
		return nil, nil, err
	}

	var allCollectorResults []CollectorResult
	results, err := slices.MapErr(loadResults, func(lr *LoadResult, _ int) (*GenerateResult, error) {
		c := collector{}
		ptestCollected := c.CollectSuiteSpecs(lr.Ptest)
		if len(ptestCollected.Errs) > 0 {
			return nil, ptestCollected.Errs[0].Err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.Pxtest)
		if len(pxtestCollected.Errs) > 0 {
			return nil, pxtestCollected.Errs[0].Err
		}

		allCollectorResults = append(allCollectorResults, ptestCollected, pxtestCollected)

		ptestSpec, err := c.ApplyTestSuiteSpecs(ptestCollected)
		if err != nil {
			return nil, err
		}

		pxtestSpec, err := c.ApplyTestSuiteSpecs(pxtestCollected)
		if err != nil {
			return nil, err
		}

		r := renderer{}
		ptestBuf, err := r.RenderTestSuiteSpec(lr.Ptest, ptestSpec)
		if err != nil {
			return nil, err
		}
		pxtestBuf, err := r.RenderTestSuiteSpec(lr.Pxtest, pxtestSpec)
		if err != nil {
			return nil, err
		}
		return &GenerateResult{AbsPath: lr.PkgDir, Package: lr.PkgPath, PTest: ptestBuf, PXTest: pxtestBuf}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return results, allCollectorResults, nil
}

