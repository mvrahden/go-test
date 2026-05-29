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

// FixtureViewModel is the view model passed to the fixture template.
type FixtureViewModel struct {
	Identifier          string
	QualifiedIdentifier string // "pkg.Name" for cross-package, "Name" for same
	ParentIdentifier    string // Identifier of the parent fixture (empty for roots, backward compat)
	ParentIdentifiers   []string // all parent fixture identifiers
	ParentFieldName     string // field name for single parent (backward compat)
	ParentFields        map[string]string // parent identifier → field name in this fixture's struct
	DependsOn           []string // same as ParentIdentifiers, for template convenience
	PkgPath             string // import path, empty if same package
	HasConfig           bool
	BeforeAll           bool
	AfterAll            bool
	BeforeEach          bool
	AfterEach           bool
	HasHydrate          bool
	HasDehydrate        bool
	ChildSuites         []*gotestast.TestSuiteSpec
	ChildFixtures       []*FixtureViewModel
	SharedFixtures      []SharedFixtureRef
}

// FlatFixtureSuite describes a suite with its full fixture ancestry chain,
// used for generating per-suite Test functions at arbitrary tree depth.
type FlatFixtureSuite struct {
	Suite         *gotestast.TestSuiteSpec
	Fixture       *FixtureViewModel   // the fixture this suite is directly bound to (first fixture)
	FixtureChain  []*FixtureViewModel // root → ... → parent fixture (full path, backward compat)
	FixtureOrder  []*FixtureViewModel // topological order of ALL needed fixtures
	FixtureFields map[string]string   // fixture identifier → field name in suite struct
}

// SharedFixtureRef describes a shared fixture embedded in a package fixture.
type SharedFixtureRef struct {
	LocalVar      string // e.g. "sf0"
	QualifiedType string // e.g. "fixtures.PostgresSharedFixture"
	FieldName     string // e.g. "PostgresSharedFixture"
	StateKey      string // e.g. "github.com/example/fixtures.PostgresSharedFixture"
	HasHydrate    bool
	HasDehydrate  bool
	PkgPath       string // import path, empty if same package
}

type headerImport struct {
	Name string
	Path string
}

type renderer struct{}

func (r renderer) RenderTestSuiteSpec(pkg *packages.Package, spec SpecOutcome, resolved *ResolveResult) ([]byte, error) {
	if pkg == nil {
		return nil, nil
	}
	if len(spec.EffectiveTestSuites) == 0 {
		return nil, nil
	}

	fixtureBound := resolved.FixtureBound
	standalone := resolved.Standalone
	allViewModels := buildAllFixtureViewModels(resolved.AllFixtures)
	hasFixtures := len(resolved.RootFixtures) > 0

	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec, hasFixtures, resolved.SuiteSharedFixtures, allViewModels); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}

	if len(fixtureBound) > 0 {
		if err := r.renderFixtures(buf, fixtureBound, allViewModels, resolved.SuiteFixtureFields); err != nil {
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

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, hasFixtures bool, suiteSharedFixtures map[string][]SharedFixtureRef, allViewModels []*FixtureViewModel) error {
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []headerImport
	}
	imports := []headerImport{
		{Path: "testing"},
		{Path: about.Repo + "/pkg/gotest"},
	}
	hasSuiteSharedFixtures := len(suiteSharedFixtures) > 0
	if hasFixtures {
		imports = append(imports, headerImport{Path: about.Repo + "/pkg/gotestruntime"})
		imports = append(imports, headerImport{Path: "context"})
		imports = append(imports, headerImport{Path: "os"})
		imports = append(imports, headerImport{Path: "time"})
	}
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.IsMethodParallel()
	}) {
		imports = append(imports, headerImport{Path: "sync"})
	}
	if !hasFixtures && hasSuiteSharedFixtures {
		imports = append(imports, headerImport{Path: "context"})
		imports = append(imports, headerImport{Path: "os"})
	}
	hasSharedFixtures := hasSuiteSharedFixtures
	seenPkg := map[string]bool{}
	for _, vm := range allViewModels {
		if vm.PkgPath != "" && !seenPkg[vm.PkgPath] {
			imports = append(imports, headerImport{Path: vm.PkgPath})
			seenPkg[vm.PkgPath] = true
		}
		for _, sf := range vm.SharedFixtures {
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
	for _, ts := range spec.EffectiveTestSuites {
		if pkgPath := ts.ContextTypePkgPath(); pkgPath != "" && !seenPkg[pkgPath] {
			imports = append(imports, headerImport{Path: pkgPath})
			seenPkg[pkgPath] = true
		}
	}
	if hasSharedFixtures {
		imports = append(imports, headerImport{Path: "encoding/json"})
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: pkg.Name,
		Imports:     imports,
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteSharedFixtures map[string][]SharedFixtureRef) error {
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":                spec,
		"SuiteSharedFixtures": suiteSharedFixtures,
	})
}

func (r *renderer) renderFixtures(buf *bytes.Buffer, fixtureBound []*gotestast.TestSuiteSpec, allViewModels []*FixtureViewModel, suiteFixtureFields map[string][]FixtureFieldBinding) error {
	if len(allViewModels) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"FixtureBoundSuites": fixtureBound,
		"AllFixtures":        allViewModels,
		"FlatSuites":         flattenSuitesDAG(allViewModels, suiteFixtureFields),
	})
}

func (renderer) formatOutput(buf *bytes.Buffer) ([]byte, error) {
	srcs, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed formatting the generated sources. err: %w", err)
	}
	return srcs, nil
}

func resolvedToFlatViewModel(rf *ResolvedFixture) *FixtureViewModel {
	vm := &FixtureViewModel{
		Identifier:          rf.Identifier,
		QualifiedIdentifier: rf.QualifiedType,
		ParentFieldName:     rf.ParentFieldName,
		PkgPath:             rf.PkgPath,
		HasConfig:           rf.HasConfig,
		BeforeAll:           rf.BeforeAll,
		AfterAll:            rf.AfterAll,
		BeforeEach:          rf.BeforeEach,
		AfterEach:           rf.AfterEach,
		HasHydrate:          rf.HasHydrate,
		HasDehydrate:        rf.HasDehydrate,
		ChildSuites:         rf.ChildSuites,
		SharedFixtures:      rf.SharedFixtures,
	}

	if len(rf.Parents) > 0 {
		vm.ParentFields = make(map[string]string)
		for _, p := range rf.Parents {
			vm.ParentIdentifiers = append(vm.ParentIdentifiers, p.Identifier)
			vm.ParentFields[p.Identifier] = rf.ParentFields[p]
		}
		vm.DependsOn = vm.ParentIdentifiers
		vm.ParentIdentifier = rf.Parents[0].Identifier
	}

	return vm
}

func buildAllFixtureViewModels(allFixtures []*ResolvedFixture) []*FixtureViewModel {
	var vms []*FixtureViewModel
	for _, rf := range allFixtures {
		vms = append(vms, resolvedToFlatViewModel(rf))
	}
	return vms
}

func flattenSuitesDAG(allViewModels []*FixtureViewModel, suiteFixtureFields map[string][]FixtureFieldBinding) []FlatFixtureSuite {
	vmByID := make(map[string]*FixtureViewModel)
	for _, vm := range allViewModels {
		vmByID[vm.Identifier] = vm
	}

	type suiteInfo struct {
		suite   *gotestast.TestSuiteSpec
		fixture *FixtureViewModel
	}
	var suites []suiteInfo
	for _, vm := range allViewModels {
		for _, s := range vm.ChildSuites {
			suites = append(suites, suiteInfo{suite: s, fixture: vm})
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

		needed := collectTransitiveDeps(si.suite.Identifier(), suiteFixtureFields, vmByID)

		var fixtureOrder []*FixtureViewModel
		for _, vm := range allViewModels {
			if needed[vm.Identifier] {
				fixtureOrder = append(fixtureOrder, vm)
			}
		}

		var chain []*FixtureViewModel
		if si.fixture != nil {
			chain = buildChainToRoot(si.fixture, vmByID)
		}

		result = append(result, FlatFixtureSuite{
			Suite:         si.suite,
			Fixture:       si.fixture,
			FixtureChain:  chain,
			FixtureOrder:  fixtureOrder,
			FixtureFields: fixtureFields,
		})
	}
	return result
}

func collectTransitiveDeps(suiteID string, suiteFixtureFields map[string][]FixtureFieldBinding, vmByID map[string]*FixtureViewModel) map[string]bool {
	needed := make(map[string]bool)
	bindings := suiteFixtureFields[suiteID]
	var visit func(id string)
	visit = func(id string) {
		if needed[id] {
			return
		}
		needed[id] = true
		vm := vmByID[id]
		if vm == nil {
			return
		}
		for _, parentID := range vm.ParentIdentifiers {
			visit(parentID)
		}
	}
	for _, b := range bindings {
		visit(b.FixtureIdentifier)
	}
	return needed
}

func buildChainToRoot(fixture *FixtureViewModel, vmByID map[string]*FixtureViewModel) []*FixtureViewModel {
	var chain []*FixtureViewModel
	current := fixture
	for current != nil {
		chain = append([]*FixtureViewModel{current}, chain...)
		if current.ParentIdentifier == "" {
			break
		}
		current = vmByID[current.ParentIdentifier]
	}
	return chain
}

