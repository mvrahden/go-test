package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	gotypes "go/types"
	"sort"
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
	headerTpl = template.Must(template.New("header").ParseFS(templates, "static/header.*.tpl"))
	gotestTpl = template.Must(template.New("gotest").Funcs(template.FuncMap{"dict": dict}).ParseFS(templates, "static/gotest.*.tpl"))
)

// dict builds a template argument from key/value pairs.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// SuiteFixtureSet is the part of the package's fixture DAG one suite reaches.
// Every wrapper of the suite holds exactly these nodes and wires these fields.
type SuiteFixtureSet struct {
	Nodes  []string            // DAG node names: shared fixtures, then package fixtures
	Fields []SuiteFixtureField // the suite's own fixture fields
	Order  []*BoundFixture     // package fixtures, dependencies first
}

// SuiteFixtureField wires one suite field to the variable its fixture lives in.
type SuiteFixtureField struct {
	Name string
	Var  string
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
	Named         *gotypes.Named    // the fixture type, nil when unknown
}

type headerImport struct {
	Name string
	Path string
}

type renderer struct {
	tags []string // the build tags the package was loaded under
}

// RenderTestSuiteSpec renders the generated test file for pkg. The returned
// FuzzTargetSet is the fan-out the file was rendered with — nil when the package
// fuzzes nothing; its ParamsByFunc is what the stale-corpus pre-flight
// compares each target's corpus entries against.
func (r renderer) RenderTestSuiteSpec(pkg *packages.Package, spec SpecOutcome, resolved *Binding, harvestSeeds bool) ([]byte, *FuzzTargetSet, error) { //nolint:gocritic // hugeParam: stable API
	if pkg == nil {
		return nil, nil, nil
	}
	if len(spec.EffectiveTestSuites) == 0 {
		return nil, nil, nil
	}

	allFixtures := resolved.AllFixtures
	sfNodeVMs := buildSharedFixtureNodeVMs(resolved.RequiredSharedFixtures)
	hasFixtures := len(resolved.RootFixtures) > 0 || len(sfNodeVMs) > 0

	// Every harness references gotestruntime: the ƒƒ_GOTEST_exec sentinel takes a
	// gotestruntime.TestCase, and each Test function runs its methods there.
	imps := newImportSet(pkg.PkgPath)
	imps.std("testing")
	imps.std(gotestImport)
	imps.std(about.Repo + "/pkg/gotestruntime")
	if hasFixtures {
		if usesContext(allFixtures, sfNodeVMs) {
			imps.std("context")
		}
		imps.std("time")
	}

	// Fixture packages take their aliases in path order, so a/pg is ƒpg0.
	var fixturePkgs []*gotypes.Package
	for _, rf := range allFixtures {
		fixturePkgs = append(fixturePkgs, rf.Named.Obj().Pkg())
	}
	for _, sf := range sfNodeVMs {
		if sf.Named != nil {
			fixturePkgs = append(fixturePkgs, sf.Named.Obj().Pkg())
		}
	}
	sort.Slice(fixturePkgs, func(i, j int) bool { return fixturePkgs[i].Path() < fixturePkgs[j].Path() })
	for _, p := range fixturePkgs {
		if p.Path() != pkg.PkgPath {
			imps.use(p.Path(), p.Name())
		}
	}

	// Resolved before anything is written: a non-fuzzable argument type is a
	// generation-time refusal, not a half-written file.
	fans, err := buildFuzzTargets(pkg, spec.EffectiveTestSuites, imps)
	if err != nil {
		return nil, nil, err
	}

	refs := typeRefs{
		Fixtures: map[string]string{},
		Shared:   map[string]string{},
		Contexts: map[string]string{},
	}
	for _, rf := range allFixtures {
		refs.Fixtures[rf.Identifier] = imps.typeRef(rf.Named)
	}
	for _, sf := range sfNodeVMs {
		refs.Shared[sf.Identifier] = sf.QualifiedType
		if sf.Named != nil {
			refs.Shared[sf.Identifier] = imps.typeRef(sf.Named)
		}
	}
	for _, ts := range spec.EffectiveTestSuites {
		if ts.HasReturningBeforeEach() {
			refs.Contexts[ts.Identifier()] = contextTypeRef(pkg, ts, imps)
		}
	}

	suiteFixtures := buildSuiteFixtureSets(resolved, sfNodeVMs)

	buf := bytes.NewBuffer(nil)
	if err := r.renderFixtures(buf, allFixtures, sfNodeVMs, refs); err != nil {
		return nil, nil, fmt.Errorf("failed rendering fixtures. err: %w", err)
	}

	if err := r.renderTestSuites(buf, spec, suiteFixtures, refs); err != nil {
		return nil, nil, fmt.Errorf("failed rendering test suites. err: %w", err)
	}

	if err := r.renderBenchSuites(buf, spec, suiteFixtures); err != nil {
		return nil, nil, fmt.Errorf("failed rendering benchmark suites. err: %w", err)
	}
	if err := r.renderFuzzSuites(buf, pkg, spec, suiteFixtures, harvestSeeds, fans); err != nil {
		return nil, nil, fmt.Errorf("failed rendering fuzz suites. err: %w", err)
	}

	// The header goes last in: only now is every import known.
	head := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(head, pkg, imps); err != nil {
		return nil, nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}
	head.Write(buf.Bytes())

	out, err := r.formatOutput(head)
	if err != nil {
		return nil, fans, err
	}
	return withBuildTags(out, r.tags), fans, nil
}

// typeRefs name the types the templates write out, each through the
// generated file's own imports.
type typeRefs struct {
	Fixtures map[string]string // package fixture identifier → type
	Shared   map[string]string // shared fixture node identifier → type
	Contexts map[string]string // suite identifier → BeforeEach's returned type
}

// contextTypeRef names the type ts's BeforeEach returns.
func contextTypeRef(pkg *packages.Package, ts *gotestast.TestSuiteSpec, imps *importSet) string {
	if pkg.Types != nil {
		if tn, ok := pkg.Types.Scope().Lookup(ts.Identifier()).(*gotypes.TypeName); ok {
			obj, _, _ := gotypes.LookupFieldOrMethod(gotypes.NewPointer(tn.Type()), true, pkg.Types, "BeforeEach")
			if fn, ok := obj.(*gotypes.Func); ok {
				if sig, ok := fn.Type().(*gotypes.Signature); ok && sig.Results().Len() == 1 {
					return imps.typeRef(sig.Results().At(0).Type())
				}
			}
		}
	}
	return ts.ContextTypeName()
}

// usesContext reports whether the fixture section names context: every package
// fixture node does, a shared fixture node only for Hydrate or Dehydrate.
func usesContext(allFixtures []*BoundFixture, sfNodes []*SharedFixtureNodeVM) bool {
	if len(allFixtures) > 0 {
		return true
	}
	for _, sf := range sfNodes {
		if sf.HasHydrate || sf.HasDehydrate {
			return true
		}
	}
	return false
}

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, imps *importSet) error {
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []headerImport
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: pkg.Name,
		Imports:     imps.imports(),
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

// Every wrapper template gets the same per-suite fixture sets, so each kind of
// wrapper holds and wires a suite's fixtures the same way.

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteFixtures map[string]SuiteFixtureSet, refs typeRefs) error { //nolint:gocritic // hugeParam: stable API
	if len(spec.EffectiveTestSuites) == 0 && len(spec.SkippedTestSuites) == 0 {
		return nil
	}
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":         spec,
		"Fixtures":     suiteFixtures,
		"ContextTypes": refs.Contexts,
	})
}

func (r *renderer) renderBenchSuites(buf *bytes.Buffer, spec SpecOutcome, suiteFixtures map[string]SuiteFixtureSet) error { //nolint:gocritic // hugeParam: stable API
	return gotestTpl.ExecuteTemplate(buf, "gotest.bench.tpl", map[string]any{
		"Spec":     spec,
		"Fixtures": suiteFixtures,
	})
}

func (r *renderer) renderFuzzSuites(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, suiteFixtures map[string]SuiteFixtureSet, harvestSeeds bool, fans *FuzzTargetSet) error { //nolint:gocritic // hugeParam: stable API
	harvested, err := harvestedSeedsForTemplate(pkg, spec, harvestSeeds)
	if err != nil {
		return err
	}
	// Passed as flat values rather than the set itself, so the template
	// never has to dereference a nil *FuzzTargetSet.
	targets := map[string]string{}
	var fanSource string
	if fans != nil {
		for _, ref := range fans.Targets {
			targets[ref.FuncName] = ref.Expr
		}
		fanSource = fans.Source
	}
	return gotestTpl.ExecuteTemplate(buf, "gotest.fuzz.tpl", map[string]any{
		"Spec":           spec,
		"Fixtures":       suiteFixtures,
		"HarvestedSeeds": harvested,
		"FuzzTargets":    targets,
		"FuzzFanSource":  fanSource,
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

func (r *renderer) renderFixtures(buf *bytes.Buffer, allFixtures []*BoundFixture, sfNodes []*SharedFixtureNodeVM, refs typeRefs) error { //nolint:gocritic // hugeParam: stable API
	if len(allFixtures) == 0 && len(sfNodes) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"AllFixtures":        allFixtures,
		"SharedFixtureNodes": sfNodes,
		"FixtureTypes":       refs.Fixtures,
		"SharedTypes":        refs.Shared,
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
		id := sharedFixtures[i].nodeID()
		stateKeyToID[sharedFixtures[i].PkgPath+"."+sharedFixtures[i].Identifier] = id
	}

	var vms []*SharedFixtureNodeVM
	for i := range sharedFixtures {
		sf := &sharedFixtures[i]
		id := sf.nodeID()
		qualifiedType := sf.QualifiedType

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
			Named:         sf.Named,
		})
	}
	return vms
}

// buildSuiteFixtureSets gives each suite the part of the fixture DAG it
// reaches: its package fixtures and their parents, and every shared fixture
// those or the suite name. Holding no more than that keeps a fixture the suite
// never uses from costing it time, or failing it.
func buildSuiteFixtureSets(resolved *Binding, sfNodes []*SharedFixtureNodeVM) map[string]SuiteFixtureSet {
	rfByID := make(map[string]*BoundFixture, len(resolved.AllFixtures))
	for _, rf := range resolved.AllFixtures {
		rfByID[rf.Identifier] = rf
	}
	suiteIDs := map[string]bool{}
	for id := range resolved.SuiteFixtureFields {
		suiteIDs[id] = true
	}
	for id := range resolved.SuiteSharedFixtures {
		suiteIDs[id] = true
	}

	sets := make(map[string]SuiteFixtureSet, len(suiteIDs))
	for id := range suiteIDs {
		var set SuiteFixtureSet
		sharedKeys := map[string]bool{}
		for _, key := range resolved.SuiteRequiredSharedFixtureKeys[id] {
			sharedKeys[key] = true
		}
		for _, sf := range sfNodes {
			if sharedKeys[sf.StateKey] {
				set.Nodes = append(set.Nodes, sf.Identifier)
			}
		}
		needed := collectTransitiveDepsRF(id, resolved.SuiteFixtureFields, rfByID)
		for _, rf := range resolved.AllFixtures {
			if needed[rf.Identifier] {
				set.Nodes = append(set.Nodes, rf.Identifier)
				set.Order = append(set.Order, rf)
			}
		}
		for _, b := range resolved.SuiteFixtureFields[id] {
			set.Fields = append(set.Fields, SuiteFixtureField{Name: b.FieldName, Var: "ƒ_" + b.FixtureIdentifier})
		}
		for _, ref := range resolved.SuiteSharedFixtures[id] {
			set.Fields = append(set.Fields, SuiteFixtureField{Name: ref.FieldName, Var: "ƒ_sf_" + ref.Identifier})
		}
		sets[id] = set
	}
	return sets
}
