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
	Identifier     string
	BeforeAll      bool
	AfterAll       bool
	BeforeEach     bool
	AfterEach      bool
	ChildSuites    []*gotestast.TestSuiteSpec
	ChildFixtures  []*FixtureViewModel
	SharedFixtures []SharedFixtureRef
}

// SharedFixtureRef describes a shared fixture embedded in a package fixture.
type SharedFixtureRef struct {
	LocalVar      string            // e.g. "pgFixture"
	QualifiedType string            // e.g. "fixtures.PostgresFixture"
	FieldName     string            // e.g. "PostgresFixture"
	EnvTags       map[string]string // field -> env var
}

type renderer struct{}

func (r renderer) RenderTestSuiteSpec(pkg *packages.Package, spec SpecOutcome) ([]byte, error) {
	if pkg == nil {
		return nil, nil
	}
	if len(spec.EffectiveTestSuites) == 0 {
		return nil, nil
	}
	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkg, spec); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}

	// Split suites into fixture-bound and standalone
	fixtureBound, standalone := splitSuitesByFixture(spec)

	if len(fixtureBound) > 0 {
		if err := r.renderFixtures(buf, spec, fixtureBound); err != nil {
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

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkg *packages.Package, spec SpecOutcome) error {
	type Import struct {
		Name string
		Path string
	}
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []Import
	}
	imports := []Import{
		{Path: "testing"},
		{Path: about.Repo + "/pkg/gotest"},
	}
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.HasParallelTestCases()
	}) {
		imports = append(imports, Import{Path: "sync"})
	}
	if len(spec.Fixtures) > 0 {
		imports = append(imports, Import{Path: "os"})
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: pkg.Name,
		Imports:     imports,
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, spec SpecOutcome) error {
	type TplData struct{}
	data := TplData{}
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec": spec,
		"Data": data,
	})
}

func (r *renderer) renderFixtures(buf *bytes.Buffer, spec SpecOutcome, fixtureBound []*gotestast.TestSuiteSpec) error {
	// Build fixture view models from the spec's fixtures and fixture-bound suites
	viewModels := buildFixtureViewModels(spec.Fixtures, fixtureBound)
	if len(viewModels) == 0 {
		return nil
	}

	// Collect all fixture-bound suites for wrapper struct generation
	return gotestTpl.ExecuteTemplate(buf, "gotest.fixture.tpl", map[string]any{
		"RootFixtures":     viewModels,
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

// splitSuitesByFixture splits effective test suites into fixture-bound and standalone.
func splitSuitesByFixture(spec SpecOutcome) (fixtureBound, standalone []*gotestast.TestSuiteSpec) {
	for _, ts := range spec.EffectiveTestSuites {
		if ts.Fixture() != nil {
			fixtureBound = append(fixtureBound, ts)
		} else {
			standalone = append(standalone, ts)
		}
	}
	return
}

// buildFixtureViewModels constructs the fixture tree view model from flat lists
// of fixtures and fixture-bound suites.
func buildFixtureViewModels(fixtures []*gotestast.FixtureSpec, fixtureBound []*gotestast.TestSuiteSpec) []*FixtureViewModel {
	// Build a map of fixture identifier -> view model
	vmMap := make(map[string]*FixtureViewModel)
	for _, f := range fixtures {
		if f.Kind != gotestast.PackageFixture {
			continue
		}
		vm := &FixtureViewModel{
			Identifier: f.Identifier(),
			BeforeAll:  f.BeforeAll != nil,
			AfterAll:   f.AfterAll != nil,
			BeforeEach: f.BeforeEach != nil,
			AfterEach:  f.AfterEach != nil,
		}
		vmMap[f.Identifier()] = vm
	}

	// Assign child suites to their fixture
	for _, ts := range fixtureBound {
		fix := ts.Fixture()
		if fix == nil {
			continue
		}
		vm, ok := vmMap[fix.Identifier()]
		if !ok {
			continue
		}
		vm.ChildSuites = append(vm.ChildSuites, ts)
	}

	// Build parent-child fixture relationships
	for _, f := range fixtures {
		if f.Kind != gotestast.PackageFixture {
			continue
		}
		if f.ParentFixture == nil {
			continue
		}
		childVM, ok := vmMap[f.Identifier()]
		if !ok {
			continue
		}
		parentVM, ok := vmMap[f.ParentFixture.Identifier()]
		if !ok {
			continue
		}
		parentVM.ChildFixtures = append(parentVM.ChildFixtures, childVM)
	}

	// Collect root fixtures (those without a parent fixture)
	var roots []*FixtureViewModel
	for _, f := range fixtures {
		if f.Kind != gotestast.PackageFixture {
			continue
		}
		if f.ParentFixture != nil {
			continue
		}
		vm := vmMap[f.Identifier()]
		roots = append(roots, vm)
	}

	return roots
}
