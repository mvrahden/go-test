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
}

type headerImport struct {
	Name string
	Path string
}

type renderer struct{}

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

	// Resolved before anything is written: a non-fuzzable argument type is a
	// generation-time refusal, not a half-written file.
	fans, err := BuildFuzzTargets(pkg, spec.EffectiveTestSuites)
	if err != nil {
		return nil, nil, err
	}

	suiteFixtures := buildSuiteFixtureSets(resolved, sfNodeVMs)

	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec, hasFixtures, resolved.SuiteSharedFixtures, allFixtures, sfNodeVMs, fans); err != nil {
		return nil, nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}

	if err := r.renderFixtures(buf, allFixtures, sfNodeVMs); err != nil {
		return nil, nil, fmt.Errorf("failed rendering fixtures. err: %w", err)
	}

	if err := r.renderTestSuites(buf, spec, suiteFixtures); err != nil {
		return nil, nil, fmt.Errorf("failed rendering test suites. err: %w", err)
	}

	if err := r.renderBenchSuites(buf, spec, suiteFixtures); err != nil {
		return nil, nil, fmt.Errorf("failed rendering benchmark suites. err: %w", err)
	}
	if err := r.renderFuzzSuites(buf, pkg, spec, suiteFixtures, harvestSeeds, fans); err != nil {
		return nil, nil, fmt.Errorf("failed rendering fuzz suites. err: %w", err)
	}

	out, err := r.formatOutput(buf)
	return out, fans, err
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

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, hasFixtures bool, suiteSharedFixtures map[string][]SharedFixtureRef, allFixtures []*BoundFixture, sfNodes []*SharedFixtureNodeVM, fans *FuzzTargetSet) error { //nolint:gocritic // hugeParam: stable API
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
	// independent sources (fixtures and fuzz fans), and a duplicated import
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
		if usesContext(allFixtures, sfNodes) {
			addImport("context")
		}
		addImport("time")
	}
	if fans != nil && fans.Source != "" {
		// Generated targets reference the gotestfuzz leaf helpers,
		// and the hybrid-leaf reader/writer.
		if fans.NeedsRuntime {
			addImport(fuzzRuntimeImport)
		}
		for _, path := range fans.PkgPaths {
			addImport(path)
		}
		// strings/strconv/math back the literal-rendering functions, and are
		// only pulled in when at least one was actually emitted.
		if fans.NeedsStrings {
			addImport("strings")
		}
		if fans.NeedsStrconv {
			addImport("strconv")
		}
		if fans.NeedsMath {
			addImport("math")
		}
	}
	// This condition must stay identical to the ones guarding the ƒfailed
	// declaration in gotest.suites.tpl. A parallel suite whose every method is
	// excluded stays in EffectiveTestSuites with no TestCases, so the template
	// emits no atomic.Bool. format.Source does not type-check and would let the
	// stray import through; it is `go test` that then refuses the whole generated
	// package with "imported and not used".
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
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

// Every wrapper template gets the same per-suite fixture sets, so each kind of
// wrapper holds and wires a suite's fixtures the same way.

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteFixtures map[string]SuiteFixtureSet) error { //nolint:gocritic // hugeParam: stable API
	if len(spec.EffectiveTestSuites) == 0 && len(spec.SkippedTestSuites) == 0 {
		return nil
	}
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":     spec,
		"Fixtures": suiteFixtures,
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

func (r *renderer) renderFixtures(buf *bytes.Buffer, allFixtures []*BoundFixture, sfNodes []*SharedFixtureNodeVM) error {
	if len(allFixtures) == 0 && len(sfNodes) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"AllFixtures":        allFixtures,
		"SharedFixtureNodes": sfNodes,
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
