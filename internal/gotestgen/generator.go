package gotestgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/goversion"
	"github.com/mvrahden/go-test/internal/pkgpattern"
	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

type GenerateResults []*GenerateResult
type GenerateResult struct {
	AbsPath                        string              // absolute directory path of the package
	PkgPath                        string              // import path (e.g. "github.com/foo/bar")
	PTest                          []byte              // generated internal test source
	PXTest                         []byte              // generated external test source
	SuiteNames                     []string            // suites with a test run: test methods or fuzz seeds (e.g. "FooTestSuite")
	TestlessSuiteNames             []string            // suites in SuiteNames without a Test<Suite> function: they run for their fuzz seeds alone
	BenchSuiteNames                []string            // suite struct identifiers with >=1 effective benchmark method
	FuzzFuncsBySuite               map[string][]string // suite identifier → generated Fuzz<Suite>_<Method> func names
	FuzzParamsByFunc               map[string][]string // generated fuzz func name → corpus type of each value the engine feeds it
	FuzzDecodedFuncs               map[string]bool     // generated fuzz func names whose input is decoded from the engine's leaves
	SkippedSuiteNames              []string            // identifiers of suites excluded by focus/X_ rules
	ExclusiveSuiteNames            []string            // identifiers of suites with SuiteConfig{Exclusive: true} — dispatched alone, after the parallel bulk
	GroupBySuite                   map[string]string   // suite identifier → SuiteConfig.ExclusiveGroup, for suites in a named group
	FixtureDepSuites               []string            // test function names that depend on shared fixtures (e.g. "TestFooSuite")
	SuiteRequiredSharedFixtureKeys map[string][]string // test func name → required state keys
	StdlibTestCount                int                 // stdlib func TestX(*testing.T) declarations — reported, never run by gotest
	TestCases                      []string            // "Test<Suite>/<Method>" of every effective test method, in source order
	FuzzCases                      []string            // generated Fuzz<Suite>_<Method> wrapper names, in source order
	BenchCases                     []string            // "Benchmark<Suite>/<Method>" of every effective benchmark method, in source order
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

// packageLoadMode type-checks the matched packages from source and reads
// their dependencies from export data. A dependency's source is loaded only
// where binding needs it, see sourceLoader.
const packageLoadMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedFiles

func CollectFromLoaded(loadResults []*LoadResult) (gotestast.TestSuiteSpecSet, error) {
	var allSuites gotestast.TestSuiteSpecSet
	for _, lr := range loadResults {
		c := collector{sources: lr.sources}
		ptestCollected := c.CollectSuiteSpecs(lr.Ptest)
		if err := collectorError(lr.Ptest, ptestCollected.Errs); err != nil {
			return nil, err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.Pxtest)
		if err := collectorError(lr.Pxtest, pxtestCollected.Errs); err != nil {
			return nil, err
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
	// Errors are the diagnostics as go test prints them: the compiler's
	// report when the build ran, starting with its `# pkg [pkg.test]`
	// header, else one line per load or type error.
	Errors []string

	compilerReport bool
}

// LoadResult holds the parsed packages for a given import path,
// split into internal-test (ptest) and external-test (pxtest) packages.
type LoadResult struct {
	PkgDir       string
	PkgPath      string
	Ptest        *packages.Package
	Pxtest       *packages.Package
	hasProdFiles bool
	sources      *sourceLoader
}

func (lr *LoadResult) IsTestOnly() bool {
	return !lr.hasProdFiles
}

// InMainModule reports whether the package belongs to a main module, a
// go.work module included: the only packages generated files are written
// into. A dependency's suites still run, from an overlay.
func (lr *LoadResult) InMainModule() bool {
	for _, p := range []*packages.Package{lr.Ptest, lr.Pxtest} {
		if p != nil && p.Module != nil {
			return p.Module.Main
		}
	}
	return false
}

// loadPackages is the shared core for all package-loading variants. Matched
// packages that fail to load are returned as BrokenPackage entries: every
// package a pattern matches must end in exactly one verdict, and a load
// failure is a verdict, not an absence. Matched packages without test files
// are named in noTestFiles, for the same reason. matched names every package
// the patterns matched, in load order.
func loadPackages(targetPkgs []string, buildFlags []string) (loaded []*LoadResult, broken []BrokenPackage, noTestFiles, matched []string, err error) {
	cfg := &packages.Config{
		Mode:  packageLoadMode,
		Tests: true,
		Fset:  token.NewFileSet(),
	}
	if len(buildFlags) > 0 {
		cfg.BuildFlags = buildFlags
	}
	totalFoundPkgs, err := packages.Load(cfg, targetPkgs...)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := goversion.Check(totalFoundPkgs); err != nil {
		return nil, nil, nil, nil, err
	}
	seen := map[string]bool{}
	for _, p := range totalFoundPkgs {
		pkgPath := strings.TrimSuffix(p.PkgPath, "_test")
		if pkgPath == "" {
			pkgPath = p.ID
		}
		if !strings.HasSuffix(p.ID, ".test") && !seen[pkgPath] {
			seen[pkgPath] = true
			matched = append(matched, pkgPath)
		}
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
			// each distinct message is reported once per package. The
			// compiler's own report (`# pkg [pkg.test]` and its lines, which
			// go list relays position-less) is the one go test prints, so it
			// is kept whole and the type errors restating it are dropped.
			for _, e := range p.Errors {
				if e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# ") {
					if !bp.compilerReport {
						bp.compilerReport = true
						bp.Errors = []string{strings.TrimRight(e.Msg, "\n")}
					}
					continue
				}
				if bp.compilerReport {
					continue
				}
				msg := e.Error()
				if e.Pos == "" {
					// go test prints a position-less error bare, not as "-: msg".
					msg = e.Msg
				}
				if !brokenMsgSeen[pkgPath][msg] {
					brokenMsgSeen[pkgPath][msg] = true
					bp.Errors = append(bp.Errors, msg)
				}
			}
			if hint := missingGotestHint(bp.Errors); hint != "" && !brokenMsgSeen[pkgPath][hint] {
				brokenMsgSeen[pkgPath][hint] = true
				bp.Errors = append(bp.Errors, hint)
			}
			continue
		}
		// Packages outside any module (std, GOPATH) hold no suites,
		// but go test gives them a verdict, so they get one too.
		loadedTestPkgs = append(loadedTestPkgs, p)
	}
	broken = make([]BrokenPackage, 0, len(brokenByPath))
	for _, bp := range brokenByPath {
		broken = append(broken, *bp)
	}
	sort.Slice(broken, func(i, j int) bool { return broken[i].PkgPath < broken[j].PkgPath })
	// A package with test files has a test main, "<id>.test".
	ids := make(map[string]bool, len(totalFoundPkgs))
	for _, p := range totalFoundPkgs {
		ids[p.ID] = true
	}
	for _, p := range loadedTestPkgs {
		if p.ID == p.PkgPath && !ids[p.ID+".test"] && !strings.HasSuffix(p.ID, ".test") {
			noTestFiles = append(noTestFiles, p.PkgPath)
		}
	}
	if len(loadedTestPkgs) == 0 {
		return nil, broken, noTestFiles, matched, nil
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

	sources := newSourceLoader(buildFlags, cfg.Fset)
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
			lr = &LoadResult{PkgPath: pkgPath, PkgDir: DeterminePkgDir(p), hasProdFiles: prodPkgs[pkgPath], sources: sources}
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
	return res, broken, noTestFiles, matched, nil
}

// LoadPackages loads and groups test packages for the given target patterns.
// Broken packages are returned separately; the caller decides whether they
// abort the command or become failed-package verdicts.
func LoadPackages(targetPkgs []string, buildFlags []string) ([]*LoadResult, []BrokenPackage, error) {
	res, broken, _, _, err := loadPackages(targetPkgs, buildFlags)
	return res, broken, err
}

// LoadPackagesMatching is LoadPackages that also reports whether the
// patterns matched any package at all.
func LoadPackagesMatching(targetPkgs []string, buildFlags []string) (loaded []*LoadResult, broken []BrokenPackage, matched bool, err error) {
	loaded, broken, _, all, err := loadPackages(targetPkgs, buildFlags)
	return loaded, broken, len(all) > 0, err
}

// RunPackages is what a run loads: every matched package, each owed a verdict.
type RunPackages struct {
	Loaded []*LoadResult
	Broken []BrokenPackage
	// NoTestFiles are the matched packages without test files.
	NoTestFiles []string
	// Unmatched are the patterns that matched no package while others did.
	Unmatched []string
	// Order is every matched package in the order go test reports them.
	Order []string
}

// NoPackagesError is a run whose patterns matched no package: it would verify
// nothing, so it cannot pass.
type NoPackagesError struct {
	Patterns []string
}

func (e *NoPackagesError) Error() string {
	quoted := make([]string, len(e.Patterns))
	for i, p := range e.Patterns {
		quoted[i] = strconv.Quote(p)
	}
	return strings.Join(quoted, ", ") + " matched no packages"
}

// LoadRunPackages is LoadPackages for a run, which owes every matched package
// a verdict: it also names those without test files, and fails with a
// *NoPackagesError when the patterns match no package at all.
func LoadRunPackages(targetPkgs []string, buildFlags []string) (RunPackages, error) {
	loaded, broken, noTestFiles, matched, err := loadPackages(targetPkgs, buildFlags)
	if err != nil {
		return RunPackages{}, err
	}
	if len(matched) == 0 {
		return RunPackages{}, &NoPackagesError{Patterns: targetPkgs}
	}
	listed, unmatched := listPatterns("", targetPkgs, buildFlags)
	return RunPackages{
		Loaded:      loaded,
		Broken:      broken,
		NoTestFiles: noTestFiles,
		Unmatched:   unmatched,
		Order:       reportOrder(matched, listed),
	}, nil
}

// listPatterns expands several patterns as go test does: the packages in its
// order, and the patterns that match none, as go list warns about them. A
// lone pattern needs neither: what loaded is what it matched. Best effort,
// the result only orders and warns.
//
// dir is where go runs, "" the current directory.
func listPatterns(dir string, patterns, buildFlags []string) (listed, unmatched []string) {
	if len(patterns) < 2 {
		return nil, nil
	}
	patterns = pkgpattern.Clean(patterns)
	args := append([]string{"list", "-e", "-find", "-f", "{{.ImportPath}}"}, buildFlags...)
	cmd := exec.Command("go", append(args, patterns...)...) //nolint:gosec // G204: the go tool with the run's own patterns
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	return strings.Fields(stdout.String()), unmatchedWarnings(stderr.String())
}

// unmatchedWarnings names the patterns go list warned matched no packages.
func unmatchedWarnings(stderr string) (unmatched []string) {
	for line := range strings.Lines(stderr) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "go: warning: ")
		if !ok {
			continue
		}
		quoted, ok := strings.CutSuffix(rest, " matched no packages")
		if !ok {
			continue
		}
		if p, err := strconv.Unquote(quoted); err == nil {
			unmatched = append(unmatched, p)
		}
	}
	return unmatched
}

// UnmatchedPatterns names the patterns that match no package while others
// do, as go list warns about them; dir is where go runs, "" the current
// directory. Best effort: the result only warns.
func UnmatchedPatterns(dir string, patterns, buildFlags []string) []string {
	_, unmatched := listPatterns(dir, patterns, buildFlags)
	return unmatched
}

// PackageDir is a package a pattern matched, listed without loading it.
type PackageDir struct {
	ImportPath string
	Dir        string // "" when Err says why there is none
	Main       bool   // in a main module, see LoadResult.InMainModule
	Err        string
}

// ListPackageDirs expands patterns as go list does, without loading the
// packages: one that does not compile, e.g. under a stale generated file, is
// listed all the same. unmatched names the patterns that matched nothing.
// dir is where go runs, "" the current directory.
func ListPackageDirs(dir string, patterns, buildFlags []string) (pkgs []PackageDir, unmatched []string, err error) {
	args := append([]string{"list", "-e", "-find", "-json=ImportPath,Dir,Module,Error"}, buildFlags...)
	cmd := exec.Command("go", append(args, pkgpattern.Clean(patterns)...)...) //nolint:gosec // G204: the go tool with the command's own patterns
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var p struct {
			ImportPath string
			Dir        string
			Module     *struct{ Main bool }
			Error      *struct{ Err string }
		}
		if err := dec.Decode(&p); err != nil {
			return nil, nil, fmt.Errorf("go list: %w", err)
		}
		pd := PackageDir{ImportPath: p.ImportPath, Dir: p.Dir, Main: p.Module != nil && p.Module.Main}
		if p.Error != nil {
			pd.Err = p.Error.Err
		}
		pkgs = append(pkgs, pd)
	}
	return pkgs, unmatchedWarnings(stderr.String()), nil
}

// reportOrder orders the matched packages as go test reports them: as go
// list expanded several patterns, else as it walks a single one, path
// segment by segment.
func reportOrder(matched, listed []string) []string {
	rest := map[string]bool{}
	for _, p := range matched {
		rest[p] = true
	}
	order := make([]string, 0, len(matched))
	for _, p := range listed {
		if rest[p] {
			order = append(order, p)
			delete(rest, p)
		}
	}
	tail := slices.Filter(matched, func(p string, _ int) bool { return rest[p] })
	// "/" sorts first, as a segment ending before its sibling does.
	segments := func(p string) string { return strings.ReplaceAll(p, "/", "\x00") }
	sort.SliceStable(tail, func(i, j int) bool { return segments(tail[i]) < segments(tail[j]) })
	return append(order, tail...)
}

func GenerateFromLoaded(loadResults []*LoadResult) (GenerateResults, []SharedFixtureInfo, error) {
	return generateFromLoaded(loadResults, true)
}

// GenerateFromLoadedOpts is GenerateFromLoaded with an explicit harvestSeeds
// switch — when false, no gotest.Each/literal-call scanning happens and
// generated fuzz wrappers get no harvested f.Add(...) seed lines.
func GenerateFromLoadedOpts(loadResults []*LoadResult, harvestSeeds bool) (GenerateResults, []SharedFixtureInfo, error) {
	return generateFromLoaded(loadResults, harvestSeeds)
}

func generateFromLoaded(loadResults []*LoadResult, harvestSeeds bool) (GenerateResults, []SharedFixtureInfo, error) {
	if err := CheckRuntimeVersion(loadResults); err != nil {
		return nil, nil, err
	}
	sharedSeen := map[string]bool{}
	var allSharedFixtures []SharedFixtureInfo

	results, err := slices.MapErr(loadResults, func(lr *LoadResult, _ int) (*GenerateResult, error) {
		c := collector{sources: lr.sources}
		ptestCollected := c.CollectSuiteSpecs(lr.Ptest)
		if err := collectorError(lr.Ptest, ptestCollected.Errs); err != nil {
			return nil, err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.Pxtest)
		if err := collectorError(lr.Pxtest, pxtestCollected.Errs); err != nil {
			return nil, err
		}

		ptestSpec, err := c.ApplyTestSuiteSpecs(ptestCollected)
		if err != nil {
			return nil, err
		}
		pxtestSpec, err := c.ApplyTestSuiteSpecs(pxtestCollected)
		if err != nil {
			return nil, err
		}

		fuzzParams := map[string][]string{}
		fuzzDecoded := map[string]bool{}
		ptestBuf, ptestFixtureDeps, ptestReqKeys, err := generateForPkg(lr.Ptest, lr.sources, ptestSpec, ptestCollected, sharedSeen, &allSharedFixtures, harvestSeeds, fuzzParams, fuzzDecoded)
		if err != nil {
			return nil, locate(lr.Ptest, err)
		}
		pxtestBuf, pxtestFixtureDeps, pxtestReqKeys, err := generateForPkg(lr.Pxtest, lr.sources, pxtestSpec, pxtestCollected, sharedSeen, &allSharedFixtures, harvestSeeds, fuzzParams, fuzzDecoded)
		if err != nil {
			return nil, locate(lr.Pxtest, err)
		}

		seen := map[string]bool{}
		var suiteNames []string
		var exclusiveNames []string
		var testlessNames []string
		var caselessSkipped []string
		exclusiveSeen := map[string]bool{}
		var groupBySuite map[string]string
		for _, effective := range []gotestast.TestSuiteSpecSet{ptestSpec.EffectiveTestSuites, pxtestSpec.EffectiveTestSuites} {
			for _, s := range effective {
				id := s.Identifier()
				if seen[id] {
					continue
				}
				seen[id] = true
				// A suite runs in a test run for its test methods or its fuzz
				// seeds. Benchmark-only suites have no test run at all.
				if len(s.TestCases()) == 0 && len(s.Fuzzers()) == 0 {
					if s.DeclaresTests() {
						caselessSkipped = append(caselessSkipped, id)
					}
					continue
				}
				suiteNames = append(suiteNames, id)
				if len(s.TestCases()) == 0 && !s.DeclaresTests() {
					testlessNames = append(testlessNames, id)
				}
				if s.IsExclusive() && !exclusiveSeen[id] {
					exclusiveSeen[id] = true
					exclusiveNames = append(exclusiveNames, id)
				}
				if g := s.ExclusiveGroup(); g != "" {
					if groupBySuite == nil {
						groupBySuite = map[string]string{}
					}
					groupBySuite[id] = g
				}
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

		fuzzFuncsBySuite := map[string][]string{}
		seenFuzzFuncs := map[string]bool{}
		var fuzzCases []string
		for _, effective := range []gotestast.TestSuiteSpecSet{ptestSpec.EffectiveTestSuites, pxtestSpec.EffectiveTestSuites} {
			for _, s := range effective {
				for _, fz := range s.Fuzzers() {
					id := s.Identifier()
					name := fmt.Sprintf("Fuzz%s_%s", id, fz.Identifier())
					if seenFuzzFuncs[name] {
						continue
					}
					seenFuzzFuncs[name] = true
					fuzzFuncsBySuite[id] = append(fuzzFuncsBySuite[id], name)
					fuzzCases = append(fuzzCases, name)
				}
			}
		}

		var testCases, benchCases []string
		caseSeen := map[string]bool{}
		for _, effective := range []gotestast.TestSuiteSpecSet{ptestSpec.EffectiveTestSuites, pxtestSpec.EffectiveTestSuites} {
			for _, s := range effective {
				id := s.Identifier()
				if caseSeen[id] {
					continue
				}
				caseSeen[id] = true
				for _, m := range s.TestCases() {
					testCases = append(testCases, "Test"+id+"/"+m.Identifier())
				}
				for _, m := range s.Benchmarks() {
					benchCases = append(benchCases, "Benchmark"+id+"/"+m.Identifier())
				}
			}
		}

		skippedNames := caselessSkipped
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

		var fuzzFuncsBySuiteResult map[string][]string
		if len(fuzzFuncsBySuite) > 0 {
			fuzzFuncsBySuiteResult = fuzzFuncsBySuite
		}
		var fuzzParamsResult map[string][]string
		if len(fuzzParams) > 0 {
			fuzzParamsResult = fuzzParams
		}
		var fuzzDecodedResult map[string]bool
		if len(fuzzDecoded) > 0 {
			fuzzDecodedResult = fuzzDecoded
		}

		return &GenerateResult{
			AbsPath:                        lr.PkgDir,
			PkgPath:                        lr.PkgPath,
			PTest:                          ptestBuf,
			PXTest:                         pxtestBuf,
			SuiteNames:                     suiteNames,
			TestlessSuiteNames:             testlessNames,
			BenchSuiteNames:                benchNames,
			FuzzFuncsBySuite:               fuzzFuncsBySuiteResult,
			FuzzParamsByFunc:               fuzzParamsResult,
			FuzzDecodedFuncs:               fuzzDecodedResult,
			SkippedSuiteNames:              skippedNames,
			ExclusiveSuiteNames:            exclusiveNames,
			GroupBySuite:                   groupBySuite,
			FixtureDepSuites:               append(ptestFixtureDeps, pxtestFixtureDeps...),
			SuiteRequiredSharedFixtureKeys: mergedReqKeys,
			StdlibTestCount:                countStdlibTests(lr.Ptest, lr.Pxtest),
			TestCases:                      testCases,
			FuzzCases:                      fuzzCases,
			BenchCases:                     benchCases,
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

// generateForPkg renders one package variant. fuzzParams and fuzzDecoded are
// filled in place with the corpus shape of every fuzz target the variant
// declares and the targets that decode their input, so the internal and
// external variants of the same package accumulate into one map.
func generateForPkg(pkg *packages.Package, sources *sourceLoader, spec SpecOutcome, collected CollectorResult, sharedSeen map[string]bool, allShared *[]SharedFixtureInfo, harvestSeeds bool, fuzzParams map[string][]string, fuzzDecoded map[string]bool) ([]byte, []string, map[string][]string, error) { //nolint:gocritic // hugeParam: stable API
	if pkg == nil || len(spec.EffectiveTestSuites) == 0 {
		return nil, nil, nil, nil
	}

	resolved, err := bind(pkg, sources, spec.EffectiveTestSuites, collected.Fixtures)
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
	buf, fans, err := r.RenderTestSuiteSpec(pkg, spec, resolved, harvestSeeds)
	if fans != nil {
		maps.Copy(fuzzParams, fans.ParamsByFunc)
		maps.Copy(fuzzDecoded, fans.DecodedFuncs)
	}
	return buf, fixtureDeps, suiteReqKeys, err
}

func fixtureTreeHasSharedFixtures(roots []*BoundFixture) bool {
	for _, rf := range roots {
		if fixtureHasSharedFixtures(rf) {
			return true
		}
	}
	return false
}

func fixtureHasSharedFixtures(rf *BoundFixture) bool {
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

// missingGotestHint names the fix when a package fails because its module
// does not require gotest: the import error alone reads like a typo.
func missingGotestHint(errs []string) string {
	for _, e := range errs {
		if strings.Contains(e, "could not import github.com/mvrahden/go-test/") {
			return "this module does not require github.com/mvrahden/go-test; run: go get -tool github.com/mvrahden/go-test/cmd/gotest@latest"
		}
	}
	return ""
}
