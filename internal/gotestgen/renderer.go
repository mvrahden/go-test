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
	ParentIdentifier    string // Identifier of the parent fixture (empty for roots)
	ParentFieldName     string // field name in this fixture's struct for the parent fixture pointer
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
	Suite        *gotestast.TestSuiteSpec
	Fixture      *FixtureViewModel   // the fixture this suite is directly bound to
	FixtureChain []*FixtureViewModel // root → ... → parent fixture (full path)
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
	viewModels := buildFixtureViewModelsFromResolved(resolved.RootFixtures)
	hasFixtures := len(resolved.RootFixtures) > 0

	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec, viewModels, hasFixtures, resolved.SuiteSharedFixtures); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}

	if len(fixtureBound) > 0 {
		if err := r.renderFixtures(buf, fixtureBound, viewModels); err != nil {
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

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, viewModels []*FixtureViewModel, hasFixtures bool, suiteSharedFixtures map[string][]SharedFixtureRef) error {
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
	for _, vm := range viewModels {
		if vm.PkgPath != "" && !seenPkg[vm.PkgPath] {
			imports = append(imports, headerImport{Path: vm.PkgPath})
			seenPkg[vm.PkgPath] = true
		}
		collectFixtureImports(vm, &imports, seenPkg)
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

func collectFixtureImports(vm *FixtureViewModel, imports *[]headerImport, seenPkg map[string]bool) {
	for _, sf := range vm.SharedFixtures {
		if sf.PkgPath != "" && !seenPkg[sf.PkgPath] {
			*imports = append(*imports, headerImport{Path: sf.PkgPath})
			seenPkg[sf.PkgPath] = true
		}
	}
	for _, child := range vm.ChildFixtures {
		if child.PkgPath != "" && !seenPkg[child.PkgPath] {
			*imports = append(*imports, headerImport{Path: child.PkgPath})
			seenPkg[child.PkgPath] = true
		}
		collectFixtureImports(child, imports, seenPkg)
	}
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome, suiteSharedFixtures map[string][]SharedFixtureRef) error {
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec":                spec,
		"SuiteSharedFixtures": suiteSharedFixtures,
	})
}

func (r *renderer) renderFixtures(buf *bytes.Buffer, fixtureBound []*gotestast.TestSuiteSpec, viewModels []*FixtureViewModel) error {
	if len(viewModels) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"RootFixtures":       viewModels,
		"FixtureBoundSuites": fixtureBound,
		"AllFixtures":        flattenFixtures(viewModels),
		"FlatSuites":         flattenSuites(viewModels),
	})
}

func (renderer) formatOutput(buf *bytes.Buffer) ([]byte, error) {
	srcs, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed formatting the generated sources. err: %w", err)
	}
	return srcs, nil
}

func buildFixtureViewModelsFromResolved(roots []*ResolvedFixture) []*FixtureViewModel {
	var vms []*FixtureViewModel
	for _, rf := range roots {
		vms = append(vms, resolvedToViewModel(rf))
	}
	return vms
}

func resolvedToViewModel(rf *ResolvedFixture) *FixtureViewModel {
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
	for _, child := range rf.Children {
		childVM := resolvedToViewModel(child)
		childVM.ParentIdentifier = vm.Identifier
		vm.ChildFixtures = append(vm.ChildFixtures, childVM)
	}
	return vm
}

func flattenFixtures(roots []*FixtureViewModel) []*FixtureViewModel {
	var result []*FixtureViewModel
	var walk func(node *FixtureViewModel)
	walk = func(node *FixtureViewModel) {
		result = append(result, node)
		for _, child := range node.ChildFixtures {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return result
}

func flattenSuites(roots []*FixtureViewModel) []FlatFixtureSuite {
	var result []FlatFixtureSuite
	var walk func(node *FixtureViewModel, chain []*FixtureViewModel)
	walk = func(node *FixtureViewModel, chain []*FixtureViewModel) {
		currentChain := append(chain, node)
		for _, suite := range node.ChildSuites {
			result = append(result, FlatFixtureSuite{
				Suite:        suite,
				Fixture:      node,
				FixtureChain: append([]*FixtureViewModel(nil), currentChain...),
			})
		}
		for _, child := range node.ChildFixtures {
			walk(child, currentChain)
		}
	}
	for _, root := range roots {
		walk(root, nil)
	}
	return result
}

