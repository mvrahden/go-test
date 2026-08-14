package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"strings"
	"text/template"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

//go:embed static
var templates embed.FS

var (
	headerTpl = template.Must(template.New("header").ParseFS(templates, "static/header.*"))
	gotestTpl = template.Must(template.New("gotest").ParseFS(templates, "static/gotest.*"))
)

// FlatFixtureSuite describes a suite with its fixture dependency graph,
// used for generating per-suite Test functions.
type FlatFixtureSuite struct {
	Suite         *gotestast.TestSuiteSpec
	Fixture       *ResolvedFixture
	FixtureOrder  []*ResolvedFixture
	FixtureFields map[string]string
}

// SharedFixtureRef describes a shared fixture embedded in a package fixture.
type SharedFixtureRef struct {
	LocalVar      string // e.g. "sf0"
	QualifiedType string // e.g. "fixtures.PostgresSharedFixture"
	FieldName     string // e.g. "PostgresSharedFixture"
	StateKey      string // e.g. "github.com/example/fixtures.PostgresSharedFixture"
	Identifier    string // e.g. "PostgresSharedFixture" (same pkg) or "fixtures_PostgresSharedFixture" (cross pkg)
	HasHydrate    bool
	HasDehydrate  bool
	PkgPath       string // import path, empty if same package
}

// SharedFixtureNodeVM is the view model for rendering a shared fixture as a DAG node.
type SharedFixtureNodeVM struct {
	Identifier    string
	QualifiedType string
	StateKey      string
	HasConfig     bool
	HasHydrate    bool
	HasDehydrate  bool
	PkgPath       string
	DependsOn     []string
	ParentFields  map[string]string // parent shared fixture identifier → field name
}

type headerImport struct {
	Name string
	Path string
}

type renderer struct{}

func (r renderer) RenderTestSuiteSpec(pkg *packages.Package, spec SpecOutcome, resolved *ResolveResult, harvestSeeds bool) ([]byte, error) { //nolint:gocritic // hugeParam: stable API
	if pkg == nil {
		return nil, nil
	}
	if len(spec.EffectiveTestSuites) == 0 {
		return nil, nil
	}

	fixtureBound := resolved.FixtureBound
	standalone := resolved.Standalone
	allFixtures := resolved.AllFixtures
	sfNodeVMs := buildSharedFixtureNodeVMs(resolved.RequiredSharedFixtures)
	hasFixtures := len(resolved.RootFixtures) > 0 || len(sfNodeVMs) > 0

	// Resolved before anything is written: a non-fuzzable argument type is a
	// generation-time refusal, not a half-written file.
	codecs, err := BuildFuzzCodecs(pkg, spec.EffectiveTestSuites)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec, hasFixtures, resolved.SuiteSharedFixtures, allFixtures, sfNodeVMs, codecs); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}

	if len(fixtureBound) > 0 || len(sfNodeVMs) > 0 {
		var fixtureTestNames []string
		for _, ts := range fixtureBound {
			fixtureTestNames = append(fixtureTestNames, ts.Identifier())
		}
		for _, ts := range standalone {
			if _, hasSF := resolved.SuiteSharedFixtures[ts.Identifier()]; hasSF {
				fixtureTestNames = append(fixtureTestNames, ts.Identifier())
			}
		}
		if err := r.renderFixtures(buf, fixtureBound, allFixtures, resolved.SuiteFixtureFields, sfNodeVMs, fixtureTestNames); err != nil {
			return nil, fmt.Errorf("failed rendering fixture suites. err: %w", err)
		}
	}

	if len(standalone) > 0 || len(spec.SkippedTestSuites) > 0 {
		standaloneSpec := SpecOutcome{
			EffectiveTestSuites: standalone,
			SkippedTestSuites:   spec.SkippedTestSuites,
			SkippedTestCases:    spec.SkippedTestCases,
		}
		if err := r.renderTestSuites(buf, standaloneSpec, resolved.SuiteSharedFixtures); err != nil {
			return nil, fmt.Errorf("failed rendering test suites. err: %w", err)
		}
	}

	if err := r.renderFuzzSuites(buf, pkg, spec, resolved.SuiteSharedFixtures, allFixtures, resolved.SuiteFixtureFields, harvestSeeds, codecs); err != nil {
		return nil, fmt.Errorf("failed rendering fuzz suites. err: %w", err)
	}

	return r.formatOutput(buf)
}

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, hasFixtures bool, suiteSharedFixtures map[string][]SharedFixtureRef, allFixtures []*ResolvedFixture, sfNodes []*SharedFixtureNodeVM, codecs *FuzzCodecSet) error { //nolint:gocritic // hugeParam: stable API
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []headerImport
	}
	// Every harness references gotestruntime: the ƒƒ_GOTEST_exec sentinel takes a
	// gotestruntime.TestCase, and each Test function builds its lifecycle T there.
	imports := []headerImport{
		{Path: "testing"},
		{Path: about.Repo + "/pkg/gotest"},
		{Path: about.Repo + "/pkg/gotestruntime"},
	}
	// Every path goes through addImport: gotestruntime is reachable from two
	// independent sources (fixtures and fuzz codecs), and a duplicated import
	// line is a compile error in the generated file.
	seenPkg := map[string]bool{}
	addImport := func(path string) {
		if path == "" || seenPkg[path] {
			return
		}
		seenPkg[path] = true
		imports = append(imports, headerImport{Path: path})
	}
	if hasFixtures {
		addImport("context")
		addImport("sync/atomic")
		addImport("time")
	}
	if codecs != nil {
		// Generated codecs are built on the FuzzReader/FuzzWriter primitives.
		addImport(fuzzCodecRuntimeImport)
		for _, path := range codecs.PkgPaths {
			addImport(path)
		}
		// strings/strconv/math back the literal-rendering functions, and are
		// only pulled in when at least one was actually emitted — unlike
		// gotestruntime above, they are not needed by every codec set.
		if codecs.NeedsStrings {
			addImport("strings")
		}
		if codecs.NeedsStrconv {
			addImport("strconv")
		}
		if codecs.NeedsMath {
			addImport("math")
		}
	}
	// This condition must stay identical to the one guarding the ƒfailed
	// declaration in gotest.suites.tpl. A parallel suite whose every method is
	// excluded stays in EffectiveTestSuites with no TestCases, so the template
	// emits no atomic.Bool. format.Source does not type-check and would let the
	// stray import through; it is `go test` that then refuses the whole generated
	// package with "imported and not used".
	if !hasFixtures && slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.IsMethodParallel() && len(v.TestCases()) > 0
	}) {
		addImport("sync/atomic")
	}
	for _, rf := range allFixtures {
		addImport(rf.PkgPath)
		for _, sf := range rf.SharedFixtures {
			addImport(sf.PkgPath)
		}
	}
	for _, refs := range suiteSharedFixtures {
		for _, sf := range refs {
			addImport(sf.PkgPath)
		}
	}
	for _, sf := range sfNodes {
		if sf.PkgPath != pkg.PkgPath {
			addImport(sf.PkgPath)
		}
	}
	for _, ts := range spec.EffectiveTestSuites {
		addImport(ts.ContextTypePkgPath())
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: pkg.Name,
		Imports:     imports,
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteSharedFixtures map[string][]SharedFixtureRef) error { //nolint:gocritic // hugeParam: stable API
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":                spec,
		"SuiteSharedFixtures": suiteSharedFixtures,
	})
}

func (r *renderer) renderFuzzSuites(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, suiteSharedFixtures map[string][]SharedFixtureRef, allFixtures []*ResolvedFixture, suiteFixtureFields map[string][]FixtureFieldBinding, harvestSeeds bool, codecs *FuzzCodecSet) error { //nolint:gocritic // hugeParam: stable API
	// Reuse the exact same fixture-bound view model gotest.bench.tpl renders
	// Benchmark<Suite> from, reshaped as a map for O(1) per-suite template lookup.
	suiteFixtures := make(map[string]*FlatFixtureSuite)
	for _, fs := range flattenSuitesDAG(allFixtures, suiteFixtureFields) {
		suiteFixtures[fs.Suite.Identifier()] = &fs
	}
	harvested, err := harvestedSeedsForTemplate(pkg, spec, harvestSeeds)
	if err != nil {
		return err
	}
	// Passed as two flat values rather than the set itself, so the template
	// never has to dereference a nil *FuzzCodecSet.
	var codecRefs []FuzzCodecRef
	var codecSource string
	if codecs != nil {
		codecRefs = codecs.Codecs
		codecSource = codecs.Source
	}
	return gotestTpl.ExecuteTemplate(buf, "gotest.fuzz.tpl", map[string]any{
		"Spec":                spec,
		"SuiteSharedFixtures": suiteSharedFixtures,
		"SuiteFixtures":       suiteFixtures,
		"HarvestedSeeds":      harvested,
		"FuzzCodecs":          codecRefs,
		"FuzzCodecSource":     codecSource,
	})
}

// harvestedSeedsForTemplate computes, for each generated Fuzz<Suite>_<Method>
// func, the pre-joined comma-separated f.Add(...) argument strings for its
// harvested seed corpus. Returns nil (no-op) when harvesting is disabled or
// no fuzz methods are present.
func harvestedSeedsForTemplate(pkg *packages.Package, spec SpecOutcome, harvestSeeds bool) (map[string][]string, error) { //nolint:gocritic // hugeParam: stable API
	if !harvestSeeds {
		return nil, nil
	}
	hasFuzzers := slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return len(v.Fuzzers()) > 0
	})
	if !hasFuzzers {
		return nil, nil
	}
	seeds, err := gotestast.HarvestSeeds(pkg, spec.EffectiveTestSuites)
	if err != nil {
		return nil, fmt.Errorf("failed harvesting fuzz seeds. err: %w", err)
	}
	if len(seeds) == 0 {
		return nil, nil
	}
	out := make(map[string][]string, len(seeds))
	for funcName, literals := range seeds {
		joined := make([]string, len(literals))
		for i, lit := range literals {
			joined[i] = strings.Join(lit.Args, ", ")
		}
		out[funcName] = joined
	}
	return out, nil
}

func (r *renderer) renderFixtures(buf *bytes.Buffer, fixtureBound []*gotestast.TestSuiteSpec, allFixtures []*ResolvedFixture, suiteFixtureFields map[string][]FixtureFieldBinding, sfNodes []*SharedFixtureNodeVM, fixtureTestNames []string) error {
	if len(allFixtures) == 0 && len(sfNodes) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"FixtureBoundSuites": fixtureBound,
		"AllFixtures":        allFixtures,
		"FlatSuites":         flattenSuitesDAG(allFixtures, suiteFixtureFields),
		"SharedFixtureNodes": sfNodes,
		"FixtureTestNames":   fixtureTestNames,
	})
}

func (renderer) formatOutput(buf *bytes.Buffer) ([]byte, error) {
	srcs, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed formatting the generated sources. err: %w", err)
	}
	return srcs, nil
}

func buildSharedFixtureNodeVMs(sharedFixtures []SharedFixtureInfo) []*SharedFixtureNodeVM {
	if len(sharedFixtures) == 0 {
		return nil
	}

	stateKeyToID := make(map[string]string, len(sharedFixtures))
	for i := range sharedFixtures {
		id := sharedFixtures[i].Identifier
		if sharedFixtures[i].PkgName != "" {
			id = sharedFixtures[i].PkgName + "_" + sharedFixtures[i].Identifier
		}
		stateKeyToID[sharedFixtures[i].PkgPath+"."+sharedFixtures[i].Identifier] = id
	}

	var vms []*SharedFixtureNodeVM
	for i := range sharedFixtures {
		sf := &sharedFixtures[i]
		id := sf.Identifier
		qualifiedType := sf.QualifiedType
		if sf.PkgName != "" {
			id = sf.PkgName + "_" + sf.Identifier
		}

		var dependsOn []string
		for _, depKey := range sf.Dependencies {
			if depID, ok := stateKeyToID[depKey]; ok {
				dependsOn = append(dependsOn, depID)
			}
		}

		var parentFields map[string]string
		if len(sf.DependencyFields) > 0 {
			parentFields = make(map[string]string)
			for depKey, fieldName := range sf.DependencyFields {
				if parentID, ok := stateKeyToID[depKey]; ok {
					parentFields[parentID] = fieldName
				}
			}
		}

		vms = append(vms, &SharedFixtureNodeVM{
			Identifier:    id,
			QualifiedType: qualifiedType,
			StateKey:      sf.PkgPath + "." + sf.Identifier,
			HasConfig:     sf.HasConfig,
			HasHydrate:    sf.HasHydrate,
			HasDehydrate:  sf.HasDehydrate,
			PkgPath:       sf.PkgPath,
			DependsOn:     dependsOn,
			ParentFields:  parentFields,
		})
	}
	return vms
}

func flattenSuitesDAG(allFixtures []*ResolvedFixture, suiteFixtureFields map[string][]FixtureFieldBinding) []FlatFixtureSuite {
	rfByID := make(map[string]*ResolvedFixture)
	for _, rf := range allFixtures {
		rfByID[rf.Identifier] = rf
	}

	type suiteInfo struct {
		suite   *gotestast.TestSuiteSpec
		fixture *ResolvedFixture
	}
	var suites []suiteInfo
	for _, rf := range allFixtures {
		for _, s := range rf.ChildSuites {
			suites = append(suites, suiteInfo{suite: s, fixture: rf})
		}
	}

	seen := make(map[string]bool)
	var result []FlatFixtureSuite
	for _, si := range suites {
		if seen[si.suite.Identifier()] {
			continue
		}
		seen[si.suite.Identifier()] = true

		fixtureFields := make(map[string]string)
		bindings := suiteFixtureFields[si.suite.Identifier()]
		for _, b := range bindings {
			fixtureFields[b.FixtureIdentifier] = b.FieldName
		}

		needed := collectTransitiveDepsRF(si.suite.Identifier(), suiteFixtureFields, rfByID)

		var fixtureOrder []*ResolvedFixture
		for _, rf := range allFixtures {
			if needed[rf.Identifier] {
				fixtureOrder = append(fixtureOrder, rf)
			}
		}

		result = append(result, FlatFixtureSuite{
			Suite:         si.suite,
			Fixture:       si.fixture,
			FixtureOrder:  fixtureOrder,
			FixtureFields: fixtureFields,
		})
	}
	return result
}

func collectTransitiveDepsRF(suiteID string, suiteFixtureFields map[string][]FixtureFieldBinding, rfByID map[string]*ResolvedFixture) map[string]bool {
	needed := make(map[string]bool)
	bindings := suiteFixtureFields[suiteID]
	var visit func(id string)
	visit = func(id string) {
		if needed[id] {
			return
		}
		needed[id] = true
		rf := rfByID[id]
		if rf == nil {
			return
		}
		for _, p := range rf.Parents {
			visit(p.Identifier)
		}
	}
	for _, b := range bindings {
		visit(b.FixtureIdentifier)
	}
	return needed
}
