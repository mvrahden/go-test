package gotestgen

import (
	"sort"
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

func Generate(targetPath string, buildFlags ...string) (GenerateResults, error) {
	res, _, err := generateSrcs(targetPath, buildFlags...)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GenerateWithSharedFixtures generates test suite sources and also returns
// the deduplicated shared fixtures discovered via demand-driven resolution.
func GenerateWithSharedFixtures(targetPath string, buildFlags ...string) (GenerateResults, []SharedFixtureInfo, error) {
	return generateSrcs(targetPath, buildFlags...)
}

func Collect(targetPath string, buildFlags ...string) (gotestast.TestSuiteSpecSet, error) {
	loadResults, err := LoadPackages(targetPath, buildFlags...)
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
func LoadPackages(targetPkg string, buildFlags ...string) ([]*LoadResult, error) {
	cfg := &packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}
	if len(buildFlags) > 0 {
		cfg.BuildFlags = buildFlags
	}
	totalFoundPkgs, err := packages.Load(cfg, targetPkg)
	if err != nil {
		return nil, err
	}

	// filter all packages with Go-Module support, skip packages with load errors
	loadedTestPkgs := slices.Filter(totalFoundPkgs, func(item *packages.Package, index int) bool {
		if len(item.Errors) > 0 {
			return false
		}
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

func generateSrcs(targetPkg string, buildFlags ...string) (GenerateResults, []SharedFixtureInfo, error) {
	loadResults, err := LoadPackages(targetPkg, buildFlags...)
	if err != nil {
		return nil, nil, err
	}

	sharedSeen := map[string]bool{}
	var allSharedFixtures []SharedFixtureInfo

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

		ptestSpec, err := c.ApplyTestSuiteSpecs(ptestCollected)
		if err != nil {
			return nil, err
		}
		pxtestSpec, err := c.ApplyTestSuiteSpecs(pxtestCollected)
		if err != nil {
			return nil, err
		}

		ptestBuf, err := generateForPkg(lr.Ptest, ptestSpec, ptestCollected, sharedSeen, &allSharedFixtures)
		if err != nil {
			return nil, err
		}
		pxtestBuf, err := generateForPkg(lr.Pxtest, pxtestSpec, pxtestCollected, sharedSeen, &allSharedFixtures)
		if err != nil {
			return nil, err
		}

		return &GenerateResult{AbsPath: lr.PkgDir, Package: lr.PkgPath, PTest: ptestBuf, PXTest: pxtestBuf}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(allSharedFixtures, func(i, j int) bool {
		if allSharedFixtures[i].PkgPath != allSharedFixtures[j].PkgPath {
			return allSharedFixtures[i].PkgPath < allSharedFixtures[j].PkgPath
		}
		return allSharedFixtures[i].Identifier < allSharedFixtures[j].Identifier
	})
	return results, allSharedFixtures, nil
}

func generateForPkg(pkg *packages.Package, spec SpecOutcome, collected CollectorResult, sharedSeen map[string]bool, allShared *[]SharedFixtureInfo) ([]byte, error) {
	if pkg == nil || len(spec.EffectiveTestSuites) == 0 {
		return nil, nil
	}

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, collected.Fixtures)
	if err != nil {
		return nil, err
	}

	for _, sf := range resolved.RequiredSharedFixtures {
		key := sf.PkgPath + "." + sf.Identifier
		if !sharedSeen[key] {
			sharedSeen[key] = true
			*allShared = append(*allShared, sf)
		}
	}

	r := renderer{}
	return r.RenderTestSuiteSpec(pkg, spec, resolved)
}

