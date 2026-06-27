package gotestgen

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

type GenerateResults []*GenerateResult
type GenerateResult struct {
	AbsPath                        string              // absolute directory path of the package
	PkgPath                        string              // import path (e.g. "github.com/foo/bar")
	PTest                          []byte              // generated internal test source
	PXTest                         []byte              // generated external test source
	SuiteNames                     []string            // suite struct identifiers (e.g. "FooTestSuite")
	SkippedSuiteNames              []string            // identifiers of suites excluded by focus/X_ rules
	FixtureDepSuites               []string            // test function names that depend on shared fixtures (e.g. "TestFooSuite")
	SuiteRequiredSharedFixtureKeys map[string][]string // test func name → required state keys
}

const (
	packageEvalMode   = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	discoveryEvalMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedFiles
)

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
	PkgDir       string
	PkgPath      string
	Ptest        *packages.Package
	Pxtest       *packages.Package
	hasProdFiles bool
}

func (lr *LoadResult) IsTestOnly() bool {
	return !lr.hasProdFiles
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

	prodPkgs := make(map[string]bool)
	for _, p := range loadedTestPkgs {
		if strings.Contains(p.ID, "[") || strings.HasSuffix(p.ID, ".test") {
			continue
		}
		for _, f := range p.GoFiles {
			if !strings.HasSuffix(f, "_test.go") {
				prodPkgs[p.PkgPath] = true
				break
			}
		}
	}

	loadedTestPkgs = slices.Filter(loadedTestPkgs, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	pkgOrder := map[string]int{}
	for i, p := range totalFoundPkgs {
		path := strings.TrimSuffix(p.PkgPath, "_test")
		if _, exists := pkgOrder[path]; !exists {
			pkgOrder[path] = i
		}
	}

	testPkgMap := map[string]*LoadResult{}
	var res []*LoadResult
	for _, p := range loadedTestPkgs {
		isPxTest := strings.HasSuffix(p.Name, "_test")
		pkgPath := p.PkgPath
		if isPxTest {
			pkgPath = strings.TrimSuffix(pkgPath, "_test")
		}
		lr, ok := testPkgMap[pkgPath]
		if !ok {
			lr = &LoadResult{PkgPath: pkgPath, PkgDir: DeterminePkgDir(p), hasProdFiles: prodPkgs[pkgPath]}
			testPkgMap[pkgPath] = lr
			res = append(res, lr)
		}
		if !isPxTest {
			lr.Ptest = p
		} else {
			lr.Pxtest = p
		}
	}
	sort.SliceStable(res, func(i, j int) bool {
		return pkgOrder[res[i].PkgPath] < pkgOrder[res[j].PkgPath]
	})
	return res, warnings, nil
}

// LoadPackages loads and groups test packages for the given target patterns.
func LoadPackages(targetPkgs []string, buildFlags []string) ([]*LoadResult, error) {
	res, _, err := loadPackages(packageEvalMode, targetPkgs, buildFlags, false)
	return res, err
}

// LoadPackagesForDiscovery loads packages using a lightweight mode without
// NeedDeps, avoiding type-checking of the entire transitive dependency graph.
func LoadPackagesForDiscovery(targetPkgs []string, buildFlags []string) ([]*LoadResult, []LoadWarning, error) {
	return loadPackages(discoveryEvalMode, targetPkgs, buildFlags, true)
}

func GenerateFromLoaded(loadResults []*LoadResult) (GenerateResults, []SharedFixtureInfo, error) {
	return generateFromLoaded(loadResults)
}

func generateFromLoaded(loadResults []*LoadResult) (GenerateResults, []SharedFixtureInfo, error) {
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

		ptestBuf, ptestFixtureDeps, ptestReqKeys, err := generateForPkg(lr.Ptest, ptestSpec, ptestCollected, sharedSeen, &allSharedFixtures)
		if err != nil {
			return nil, err
		}
		pxtestBuf, pxtestFixtureDeps, pxtestReqKeys, err := generateForPkg(lr.Pxtest, pxtestSpec, pxtestCollected, sharedSeen, &allSharedFixtures)
		if err != nil {
			return nil, err
		}

		seen := map[string]bool{}
		var suiteNames []string
		for _, s := range ptestSpec.EffectiveTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				suiteNames = append(suiteNames, id)
			}
		}
		for _, s := range pxtestSpec.EffectiveTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				suiteNames = append(suiteNames, id)
			}
		}

		var skippedNames []string
		for _, s := range ptestSpec.SkippedTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				skippedNames = append(skippedNames, id)
			}
		}
		for _, s := range pxtestSpec.SkippedTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				skippedNames = append(skippedNames, id)
			}
		}

		// Merge per-suite required shared fixture keys from both test suffixes.
		var mergedReqKeys map[string][]string
		if len(ptestReqKeys) > 0 || len(pxtestReqKeys) > 0 {
			mergedReqKeys = make(map[string][]string, len(ptestReqKeys)+len(pxtestReqKeys))
			maps.Copy(mergedReqKeys, ptestReqKeys)
			maps.Copy(mergedReqKeys, pxtestReqKeys)
		}

		return &GenerateResult{
			AbsPath:                        lr.PkgDir,
			PkgPath:                        lr.PkgPath,
			PTest:                          ptestBuf,
			PXTest:                         pxtestBuf,
			SuiteNames:                     suiteNames,
			SkippedSuiteNames:              skippedNames,
			FixtureDepSuites:               append(ptestFixtureDeps, pxtestFixtureDeps...),
			SuiteRequiredSharedFixtureKeys: mergedReqKeys,
		}, nil
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

func generateForPkg(pkg *packages.Package, spec SpecOutcome, collected CollectorResult, sharedSeen map[string]bool, allShared *[]SharedFixtureInfo) ([]byte, []string, map[string][]string, error) { //nolint:gocritic // hugeParam: stable API
	if pkg == nil || len(spec.EffectiveTestSuites) == 0 {
		return nil, nil, nil, nil
	}

	resolved, err := Resolve(pkg, spec.EffectiveTestSuites, collected.Fixtures)
	if err != nil {
		return nil, nil, nil, err
	}

	for i := range resolved.RequiredSharedFixtures {
		key := resolved.RequiredSharedFixtures[i].PkgPath + "." + resolved.RequiredSharedFixtures[i].Identifier
		if !sharedSeen[key] {
			sharedSeen[key] = true
			*allShared = append(*allShared, resolved.RequiredSharedFixtures[i])
		}
	}

	var fixtureDeps []string
	for id, refs := range resolved.SuiteSharedFixtures {
		if len(refs) > 0 {
			fixtureDeps = append(fixtureDeps, "Test"+id)
		}
	}
	if fixtureTreeHasSharedFixtures(resolved.RootFixtures) {
		seen := make(map[string]bool, len(fixtureDeps))
		for _, d := range fixtureDeps {
			seen[d] = true
		}
		for _, ts := range resolved.FixtureBound {
			name := "Test" + ts.Identifier()
			if !seen[name] {
				fixtureDeps = append(fixtureDeps, name)
			}
		}
	}

	// Convert suite-identifier-keyed required keys to test-func-name-keyed.
	var suiteReqKeys map[string][]string
	if len(resolved.SuiteRequiredSharedFixtureKeys) > 0 {
		suiteReqKeys = make(map[string][]string, len(resolved.SuiteRequiredSharedFixtureKeys))
		for suiteID, keys := range resolved.SuiteRequiredSharedFixtureKeys {
			suiteReqKeys["Test"+suiteID] = keys
		}
	}

	r := renderer{}
	buf, err := r.RenderTestSuiteSpec(pkg, spec, resolved)
	return buf, fixtureDeps, suiteReqKeys, err
}

func fixtureTreeHasSharedFixtures(roots []*ResolvedFixture) bool {
	for _, rf := range roots {
		if fixtureHasSharedFixtures(rf) {
			return true
		}
	}
	return false
}

func fixtureHasSharedFixtures(rf *ResolvedFixture) bool {
	if len(rf.SharedFixtures) > 0 {
		return true
	}
	for _, child := range rf.Children {
		if fixtureHasSharedFixtures(child) {
			return true
		}
	}
	return false
}
