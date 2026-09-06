package gotestgen

import (
	"go/ast"
	"maps"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
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
	BenchSuiteNames                []string            // suite struct identifiers with >=1 effective benchmark method
	SkippedSuiteNames              []string            // identifiers of suites excluded by focus/X_ rules
	ExclusiveSuiteNames            []string            // identifiers of suites with SuiteConfig{Exclusive: true} — dispatched alone, after the parallel bulk
	FixtureDepSuites               []string            // test function names that depend on shared fixtures (e.g. "TestFooSuite")
	SuiteRequiredSharedFixtureKeys map[string][]string // test func name → required state keys
	StdlibTestCount                int                 // stdlib func TestX(*testing.T) declarations — reported, never run by gotest
}

// countStdlibTests counts top-level runnable stdlib test functions — Test*,
// Benchmark*, Fuzz* (single *testing.T/B/F param) and Example* — across the
// given package variants, excluding TestMain, non-test files, and generated
// runner files. gotest never runs these; they are counted so the runner can
// report them honestly.
func countStdlibTests(pkgs ...*packages.Package) int {
	n := 0
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Pos()).Filename
			if !strings.HasSuffix(filename, "_test.go") || about.PSuiteRegex.MatchString(filepath.Base(filename)) {
				continue
			}
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv != nil || fd.Name.Name == "TestMain" {
					continue
				}
				name := fd.Name.Name
				if strings.HasPrefix(name, "Example") {
					n++
					continue
				}
				if !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Benchmark") && !strings.HasPrefix(name, "Fuzz") {
					continue
				}
				if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 || len(fd.Type.Params.List[0].Names) > 1 {
					continue
				}
				star, ok := fd.Type.Params.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				if sel, ok := star.X.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "T", "B", "F":
						n++
					}
				}
			}
		}
	}
	return n
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

// BrokenPackage identifies a matched package that failed to load. Suite
// discovery requires a successful parse and type-check, so a package that
// fails either cannot be distinguished from one with no suites — it must
// carry a build-failure verdict of its own, never disappear from the result.
type BrokenPackage struct {
	PkgPath string
	Dir     string
	Errors  []string
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

// loadPackages is the shared core for all package-loading variants. Matched
// packages that fail to load are returned as BrokenPackage entries: every
// package a pattern matches must end in exactly one verdict, and a load
// failure is a verdict, not an absence.
func loadPackages(mode packages.LoadMode, targetPkgs []string, buildFlags []string) ([]*LoadResult, []BrokenPackage, error) {
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

	brokenByPath := map[string]*BrokenPackage{}
	brokenMsgSeen := map[string]map[string]bool{}
	var loadedTestPkgs []*packages.Package
	for _, p := range totalFoundPkgs {
		if len(p.Errors) > 0 {
			pkgPath := strings.TrimSuffix(p.PkgPath, "_test")
			if pkgPath == "" {
				pkgPath = p.ID
			}
			if strings.HasSuffix(pkgPath, ".test") {
				// Synthesized test-main package: its errors duplicate the
				// diagnostics of the variants it links.
				continue
			}
			bp := brokenByPath[pkgPath]
			if bp == nil {
				bp = &BrokenPackage{PkgPath: pkgPath, Dir: DeterminePkgDir(p)}
				brokenByPath[pkgPath] = bp
				brokenMsgSeen[pkgPath] = map[string]bool{}
			}
			// Package variants (ptest, pxtest) repeat the same diagnostics;
			// each distinct message is reported once per package.
			for _, e := range p.Errors {
				msg := e.Error()
				if !brokenMsgSeen[pkgPath][msg] {
					brokenMsgSeen[pkgPath][msg] = true
					bp.Errors = append(bp.Errors, msg)
				}
			}
			continue
		}
		if p.Module != nil {
			loadedTestPkgs = append(loadedTestPkgs, p)
		}
	}
	broken := make([]BrokenPackage, 0, len(brokenByPath))
	for _, bp := range brokenByPath {
		broken = append(broken, *bp)
	}
	sort.Slice(broken, func(i, j int) bool { return broken[i].PkgPath < broken[j].PkgPath })
	if len(loadedTestPkgs) == 0 {
		return nil, broken, nil
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
	// A package broken in one variant must not run its intact variants: the
	// test build for that package fails as a whole, so the verdict is the
	// broken entry, not a partial run.
	res = slices.Filter(res, func(lr *LoadResult, _ int) bool {
		return brokenByPath[lr.PkgPath] == nil
	})
	sort.SliceStable(res, func(i, j int) bool {
		return pkgOrder[res[i].PkgPath] < pkgOrder[res[j].PkgPath]
	})
	return res, broken, nil
}

// LoadPackages loads and groups test packages for the given target patterns.
// Broken packages are returned separately; the caller decides whether they
// abort the command or become failed-package verdicts.
func LoadPackages(targetPkgs []string, buildFlags []string) ([]*LoadResult, []BrokenPackage, error) {
	return loadPackages(packageEvalMode, targetPkgs, buildFlags)
}

// LoadPackagesForDiscovery loads packages using a lightweight mode without
// NeedDeps, avoiding type-checking of the entire transitive dependency graph.
func LoadPackagesForDiscovery(targetPkgs []string, buildFlags []string) ([]*LoadResult, []BrokenPackage, error) {
	return loadPackages(discoveryEvalMode, targetPkgs, buildFlags)
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
		var exclusiveNames []string
		exclusiveSeen := map[string]bool{}
		for _, s := range ptestSpec.EffectiveTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				suiteNames = append(suiteNames, id)
			}
			if s.IsExclusive() && !exclusiveSeen[id] {
				exclusiveSeen[id] = true
				exclusiveNames = append(exclusiveNames, id)
			}
		}
		for _, s := range pxtestSpec.EffectiveTestSuites {
			id := s.Identifier()
			if !seen[id] {
				seen[id] = true
				suiteNames = append(suiteNames, id)
			}
			if s.IsExclusive() && !exclusiveSeen[id] {
				exclusiveSeen[id] = true
				exclusiveNames = append(exclusiveNames, id)
			}
		}

		benchSeen := map[string]bool{}
		var benchNames []string
		for _, s := range ptestSpec.EffectiveTestSuites {
			if len(s.Benchmarks()) == 0 {
				continue
			}
			id := s.Identifier()
			if !benchSeen[id] {
				benchSeen[id] = true
				benchNames = append(benchNames, id)
			}
		}
		for _, s := range pxtestSpec.EffectiveTestSuites {
			if len(s.Benchmarks()) == 0 {
				continue
			}
			id := s.Identifier()
			if !benchSeen[id] {
				benchSeen[id] = true
				benchNames = append(benchNames, id)
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
			BenchSuiteNames:                benchNames,
			SkippedSuiteNames:              skippedNames,
			ExclusiveSuiteNames:            exclusiveNames,
			FixtureDepSuites:               append(ptestFixtureDeps, pxtestFixtureDeps...),
			SuiteRequiredSharedFixtureKeys: mergedReqKeys,
			StdlibTestCount:                countStdlibTests(lr.Ptest, lr.Pxtest),
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
