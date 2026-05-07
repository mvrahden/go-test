package gotestgen

import (
	"fmt"
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
	packageEvalMode    = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	discoveryEvalMode  = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedFiles
)

func Generate(targetPkgs []string, buildFlags []string) (GenerateResults, error) {
	res, _, err := generateSrcs(targetPkgs, buildFlags)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GenerateWithSharedFixtures generates test suite sources and also returns
// the deduplicated shared fixtures discovered via demand-driven resolution.
func GenerateWithSharedFixtures(targetPkgs []string, buildFlags []string) (GenerateResults, []SharedFixtureInfo, error) {
	return generateSrcs(targetPkgs, buildFlags)
}

func Collect(targetPkgs []string, buildFlags []string) (gotestast.TestSuiteSpecSet, error) {
	loadResults, err := LoadPackages(targetPkgs, buildFlags)
	if err != nil {
		return nil, err
	}
	return CollectFromLoaded(loadResults)
}

func CollectFromLoaded(loadResults []*LoadResult) (gotestast.TestSuiteSpecSet, error) {
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

// LoadWarning represents a non-fatal issue found during package loading.
type LoadWarning struct {
	PkgPath string
	Message string
}

// LoadResult holds the parsed packages for a given import path,
// split into internal-test (ptest) and external-test (pxtest) packages.
type LoadResult struct {
	PkgDir  string
	PkgPath string
	Ptest   *packages.Package
	Pxtest  *packages.Package
}

func (lr *LoadResult) IsTestOnly() bool {
	if lr.Ptest == nil {
		return true
	}
	for _, f := range lr.Ptest.GoFiles {
		if !strings.HasSuffix(f, "_test.go") {
			return false
		}
	}
	return true
}

// loadPackages is the shared core for all package-loading variants.
func loadPackages(mode packages.LoadMode, targetPkgs []string, buildFlags []string, collectWarnings bool) ([]*LoadResult, []LoadWarning, error) {
	cfg := &packages.Config{
		Mode:  mode,
		Tests: true,
	}
	if len(buildFlags) > 0 {
		cfg.BuildFlags = buildFlags
	}
	totalFoundPkgs, err := packages.Load(cfg, targetPkgs...)
	if err != nil {
		return nil, nil, err
	}

	var warnings []LoadWarning
	seen := make(map[string]bool)
	var loadedTestPkgs []*packages.Package
	for _, p := range totalFoundPkgs {
		if len(p.Errors) > 0 {
			if collectWarnings {
				pkgPath := strings.TrimSuffix(p.PkgPath, "_test")
				if strings.HasSuffix(pkgPath, ".test") {
					continue
				}
				if !seen[pkgPath] {
					seen[pkgPath] = true
					for _, e := range p.Errors {
						warnings = append(warnings, LoadWarning{PkgPath: pkgPath, Message: fmt.Sprintf("%s", e)})
					}
				}
			}
			continue
		}
		if p.Module != nil {
			loadedTestPkgs = append(loadedTestPkgs, p)
		}
	}
	if len(loadedTestPkgs) == 0 {
		return nil, warnings, nil
	}

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
	return res, warnings, nil
}

// LoadPackages loads and groups test packages for the given target patterns.
func LoadPackages(targetPkgs []string, buildFlags []string) ([]*LoadResult, error) {
	res, _, err := loadPackages(packageEvalMode, targetPkgs, buildFlags, false)
	return res, err
}

// LoadPackagesWithWarnings is like LoadPackages but also returns warnings
// for packages that were skipped due to load errors (e.g. build constraints).
func LoadPackagesWithWarnings(targetPkgs []string, buildFlags []string) ([]*LoadResult, []LoadWarning, error) {
	return loadPackages(packageEvalMode, targetPkgs, buildFlags, true)
}

// LoadPackagesForDiscovery loads packages using a lightweight mode without
// NeedDeps, avoiding type-checking of the entire transitive dependency graph.
func LoadPackagesForDiscovery(targetPkgs []string, buildFlags []string) ([]*LoadResult, []LoadWarning, error) {
	return loadPackages(discoveryEvalMode, targetPkgs, buildFlags, true)
}

func GenerateFromLoaded(loadResults []*LoadResult) (GenerateResults, []SharedFixtureInfo, error) {
	return generateSrcsFromLoaded(loadResults)
}

func generateSrcs(targetPkgs []string, buildFlags []string) (GenerateResults, []SharedFixtureInfo, error) {
	loadResults, err := LoadPackages(targetPkgs, buildFlags)
	if err != nil {
		return nil, nil, err
	}
	return generateSrcsFromLoaded(loadResults)
}

func generateSrcsFromLoaded(loadResults []*LoadResult) (GenerateResults, []SharedFixtureInfo, error) {
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

