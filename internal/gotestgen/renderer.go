package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"text/template"

	"github.com/mvrahden/go-test/about"
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

func (r renderer) RenderTestSuiteSpec(pkg *packages.Package, spec SpecOutcome, resolved *ResolveResult) ([]byte, error) { //nolint:gocritic
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

	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec, hasFixtures, resolved.SuiteSharedFixtures, allFixtures, sfNodeVMs); err != nil {
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

	return r.formatOutput(buf)
}

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, hasFixtures bool, suiteSharedFixtures map[string][]SharedFixtureRef, allFixtures []*ResolvedFixture, sfNodes []*SharedFixtureNodeVM) error { //nolint:gocritic
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []headerImport
	}
	imports := []headerImport{
		{Path: "testing"},
		{Path: about.Repo + "/pkg/gotest"},
	}
	if hasFixtures {
		imports = append(imports,
			headerImport{Path: about.Repo + "/pkg/gotestruntime"},
			headerImport{Path: "context"},
			headerImport{Path: "sync/atomic"},
			headerImport{Path: "time"},
		)
	}
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.IsMethodParallel()
	}) {
		imports = append(imports, headerImport{Path: "sync"})
	}
	seenPkg := map[string]bool{}
	for _, rf := range allFixtures {
		if rf.PkgPath != "" && !seenPkg[rf.PkgPath] {
			imports = append(imports, headerImport{Path: rf.PkgPath})
			seenPkg[rf.PkgPath] = true
		}
		for _, sf := range rf.SharedFixtures {
			if sf.PkgPath != "" && !seenPkg[sf.PkgPath] {
				imports = append(imports, headerImport{Path: sf.PkgPath})
				seenPkg[sf.PkgPath] = true
			}
		}
	}
	for _, refs := range suiteSharedFixtures {
		for _, sf := range refs {
			if sf.PkgPath != "" && !seenPkg[sf.PkgPath] {
				imports = append(imports, headerImport{Path: sf.PkgPath})
				seenPkg[sf.PkgPath] = true
			}
		}
	}
	for _, sf := range sfNodes {
		if sf.PkgPath != "" && sf.PkgPath != pkg.PkgPath && !seenPkg[sf.PkgPath] {
			imports = append(imports, headerImport{Path: sf.PkgPath})
			seenPkg[sf.PkgPath] = true
		}
	}
	for _, ts := range spec.EffectiveTestSuites {
		if pkgPath := ts.ContextTypePkgPath(); pkgPath != "" && !seenPkg[pkgPath] {
			imports = append(imports, headerImport{Path: pkgPath})
			seenPkg[pkgPath] = true
		}
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: pkg.Name,
		Imports:     imports,
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteSharedFixtures map[string][]SharedFixtureRef) error { //nolint:gocritic
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":                spec,
		"SuiteSharedFixtures": suiteSharedFixtures,
	})
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
