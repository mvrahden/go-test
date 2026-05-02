package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"strings"
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
	gotestTpl = template.Must(template.New("gotest").Funcs(tplFuncs).ParseFS(templates, "static/gotest.*"))
	tplFuncs  = map[string]any{
		"hasSuffix": strings.HasSuffix,
	}
)

// FixtureViewModel is the view model passed to the fixture template.
type FixtureViewModel struct {
	Identifier          string
	QualifiedIdentifier string // "pkg.Name" for cross-package, "Name" for same
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
	if err := r.renderFileHeader(buf, pkg, spec, viewModels, hasFixtures); err != nil {
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
		if err := r.renderTestSuites(buf, standaloneSpec); err != nil {
			return nil, fmt.Errorf("failed rendering test suites. err: %w", err)
		}
	}

	return r.formatOutput(buf)
}

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome, viewModels []*FixtureViewModel, hasFixtures bool) error {
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
		imports = append(imports, headerImport{Path: "time"})
	}
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.HasParallelTestCases()
	}) {
		imports = append(imports, headerImport{Path: "sync"})
	}
	if hasFixtures {
		imports = append(imports, headerImport{Path: "context"})
		imports = append(imports, headerImport{Path: "os"})
	}
	hasSharedFixtures := false
	seenPkg := map[string]bool{}
	for _, vm := range viewModels {
		if vm.PkgPath != "" && !seenPkg[vm.PkgPath] {
			imports = append(imports, headerImport{Path: vm.PkgPath})
			seenPkg[vm.PkgPath] = true
		}
		collectFixtureImports(vm, &imports, seenPkg, &hasSharedFixtures)
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

func collectFixtureImports(vm *FixtureViewModel, imports *[]headerImport, seenPkg map[string]bool, hasSharedFixtures *bool) {
	for _, sf := range vm.SharedFixtures {
		*hasSharedFixtures = true
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
		collectFixtureImports(child, imports, seenPkg, hasSharedFixtures)
	}
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome) error {
	type TplData struct{}
	data := TplData{}
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec": spec,
		"Data": data,
	})
}

func (r *renderer) renderFixtures(buf *bytes.Buffer, fixtureBound []*gotestast.TestSuiteSpec, viewModels []*FixtureViewModel) error {
	if len(viewModels) == 0 {
		return nil
	}

	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"RootFixtures":       viewModels,
		"FixtureBoundSuites": fixtureBound,
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
		vm.ChildFixtures = append(vm.ChildFixtures, resolvedToViewModel(child))
	}
	return vm
}

