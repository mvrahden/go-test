package pkgload_test

import (
	"fmt"
	"go/types"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/genstub"
	"github.com/mvrahden/go-test/internal/pkgload"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// LoadTestSuite compares the loader with the go/packages load it replaces,
// which builds export data for every test variant it lists.
//
// Sequential: a method replaces the package's go/packages call.
type LoadTestSuite struct {
	root      string
	stubFlags []string // a generated file left on disk loads as its package clause
	unstub    func()
	cgo       bool
}

func (s *LoadTestSuite) SuiteConfig() gotest.SuiteConfig {
	// From an empty build cache the slowest method takes about 6s; its export
	// load compiles the corpus, which a loaded CI runner stretches past 30s.
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 2 * time.Minute
	return cfg
}

const (
	testdata  = "./internal/pkgload/testdata/"
	generated = "./tests/conformance/testdata/shapes/stalerename"
	repoPath  = "github.com/mvrahden/go-test/internal/pkgload/testdata/"
)

func (s *LoadTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.root = root
	s.stubFlags, s.unstub, err = genstub.LoadFlags(root, []string{generated}, nil)
	gotest.NoError(t, err)
	out, err := exec.CommandContext(t.Context(), "go", "env", "CGO_ENABLED").Output()
	gotest.NoError(t, err)
	s.cgo = strings.TrimSpace(string(out)) == "1" // go turns it off without a C compiler
}

func (s *LoadTestSuite) AfterAll(t *gotest.T) {
	if s.unstub != nil {
		s.unstub()
	}
}

type corpusRow struct {
	Desc    string
	Dir     string // relative to the repository root
	Pattern string
	Stubbed bool
}

func (s *LoadTestSuite) corpus() []corpusRow {
	rows := []corpusRow{
		{Desc: "in-package tests only", Pattern: testdata + "inpkg"},
		{Desc: "external tests only", Pattern: testdata + "external"},
		{Desc: "in-package and external tests", Pattern: testdata + "both"},
		{Desc: "a dependency rebuilt for the test", Pattern: testdata + "rebuilt/..."},
		{Desc: "a generated file left on disk", Pattern: generated, Stubbed: true},
		{Desc: "the examples module", Dir: "examples", Pattern: "./..."},
	}
	if s.cgo {
		rows = append(rows, corpusRow{Desc: "a cgo file in the package under test", Pattern: testdata + "cgo"})
	}
	return rows
}

const exportLoadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
	packages.NeedModule | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes

func (s *LoadTestSuite) config(row corpusRow) *pkgload.Config {
	cfg := &pkgload.Config{Dir: filepath.Join(s.root, row.Dir), Tests: true}
	if row.Stubbed {
		cfg.BuildFlags = s.stubFlags
		cfg.ParseFile = about.ParseStubbingGenerated
	}
	return cfg
}

// loadWithExport is the load at the base: types through go list -export.
func (s *LoadTestSuite) loadWithExport(t *gotest.T, row corpusRow) []*packages.Package {
	cfg := s.config(row)
	pkgs, err := packages.Load(&packages.Config{
		Mode: exportLoadMode, Tests: true, Dir: cfg.Dir, BuildFlags: cfg.BuildFlags, ParseFile: cfg.ParseFile,
	}, row.Pattern)
	gotest.NoError(t, err)
	return byID(pkgs)
}

func byID(pkgs []*packages.Package) []*packages.Package {
	return slices.SortedFunc(slices.Values(pkgs), func(a, b *packages.Package) int { return strings.Compare(a.ID, b.ID) })
}

func ids(pkgs []*packages.Package) []string {
	out := make([]string, len(pkgs))
	for i, p := range pkgs {
		out[i] = p.ID
	}
	return out
}

// scopeOf renders every object the package declares.
func scopeOf(p *packages.Package) []string {
	var out []string
	for _, name := range p.Types.Scope().Names() {
		out = append(out, types.ObjectString(p.Types.Scope().Lookup(name), nil))
	}
	slices.Sort(out)
	return out
}

// usesOf renders every identifier that resolves to a package-level object.
func usesOf(p *packages.Package) []string {
	var out []string
	for id, obj := range p.TypesInfo.Uses {
		if obj.Pkg() != nil {
			out = append(out, p.Fset.Position(id.Pos()).String()+" "+obj.Pkg().Path()+"."+obj.Name())
		}
	}
	slices.Sort(out)
	return out
}

func (s *LoadTestSuite) TestMatchesTheExportLoad(t *gotest.T) {
	for t, row := range gotest.Each(t, s.corpus()) {
		want := s.loadWithExport(t, row)
		got, err := pkgload.Load(s.config(row), row.Pattern)
		gotest.NoError(t, err)
		got = byID(got)
		gotest.Equal(t, ids(want), ids(got))
		for i := range want {
			gotest.Equal(t, want[i].Name, got[i].Name)
			gotest.Equal(t, want[i].GoFiles, got[i].GoFiles, "files of %s", want[i].ID)
			gotest.Equal(t, want[i].CompiledGoFiles, got[i].CompiledGoFiles)
			gotest.Equal(t, want[i].Module, got[i].Module, "module of %s", want[i].ID)
			gotest.Equal(t, want[i].Errors, got[i].Errors, "errors of %s", want[i].ID)
			gotest.Empty(t, got[i].Errors)
			gotest.Equal(t, scopeOf(want[i]), scopeOf(got[i]), "package scope of %s", want[i].ID)
			gotest.Len(t, got[i].Syntax, len(want[i].Syntax))
			gotest.Equal(t, usesOf(want[i]), usesOf(got[i]), "resolved identifiers of %s", want[i].ID)
		}
	}
}

func (s *LoadTestSuite) TestNeverBuildsATestVariant(t *gotest.T) {
	var calls []*packages.Config
	restore := pkgload.ExportSetLoad(func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
		calls = append(calls, cfg)
		return packages.Load(cfg, patterns...)
	})
	_, err := pkgload.Load(&pkgload.Config{Dir: s.root, Tests: true}, testdata+"both")
	restore()
	gotest.NoError(t, err)
	t.It("makes two loads", func(it *gotest.T) { gotest.Len(it, calls, 2) })
	t.It("asks for no types where it asks for tests", func(it *gotest.T) {
		gotest.True(it, calls[0].Tests)
		gotest.Zero(it, calls[0].Mode&(packages.NeedTypes|packages.NeedTypesInfo|packages.NeedExportFile|packages.NeedSyntax|packages.NeedCompiledGoFiles|packages.NeedTypesSizes))
	})
	t.It("asks for no tests where it asks for export data", func(it *gotest.T) {
		gotest.False(it, calls[1].Tests)
		gotest.NotZero(it, calls[1].Mode&packages.NeedExportFile)
	})
}

// errorsOf maps each package to the kind and message of its errors, in order.
func errorsOf(pkgs []*packages.Package) map[string][]string {
	out := map[string][]string{}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			out[p.ID] = append(out[p.ID], fmt.Sprintf("%d %s", e.Kind, e.Msg))
		}
	}
	return out
}

func (s *LoadTestSuite) TestABrokenPackage(t *gotest.T) {
	for t, row := range gotest.Each(t, []corpusRow{
		{Desc: "a type error in a test file", Pattern: testdata + "broken/typeerr"},
		{Desc: "an import that does not resolve", Pattern: testdata + "broken/importerr"},
	}) {
		want := s.loadWithExport(t, row)
		got, err := pkgload.Load(s.config(row), row.Pattern)
		gotest.NoError(t, err)
		gotest.NotEmpty(t, errorsOf(want))
		gotest.Equal(t, errorsOf(want), errorsOf(got))
	}
}

func (s *LoadTestSuite) TestABrokenPackageBesideAnIntactOne(t *gotest.T) {
	var calls [][]string
	restore := pkgload.ExportSetLoad(func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
		calls = append(calls, patterns)
		return packages.Load(cfg, patterns...)
	})
	pkgs, err := pkgload.Load(&pkgload.Config{Dir: s.root, Tests: true}, testdata+"broken/typeerr", testdata+"inpkg")
	restore()
	gotest.NoError(t, err)
	t.It("loads the broken package alone once more", func(it *gotest.T) {
		gotest.Len(it, calls, 3)
		gotest.Equal(it, []string{repoPath + "broken/typeerr"}, calls[2])
	})
	t.It("leaves the intact package without errors", func(it *gotest.T) {
		var intact int
		for _, p := range pkgs {
			if strings.HasPrefix(p.ID, repoPath+"inpkg") {
				intact++
				gotest.Empty(it, p.Errors, "errors of %s", p.ID)
			}
		}
		gotest.Equal(it, 3, intact) // the package, its test variant and its test main
	})
}

func (s *LoadTestSuite) TestTheRebuiltDependency(t *gotest.T) {
	pkgs, err := pkgload.Load(&pkgload.Config{Dir: s.root, Tests: true}, testdata+"rebuilt/p")
	gotest.NoError(t, err)
	byName := map[string]*packages.Package{}
	for _, p := range pkgs {
		byName[p.ID] = p
	}
	const forTest = " [" + repoPath + "rebuilt/p.test]"
	external := byName[repoPath+"rebuilt/p_test"+forTest]
	variant := byName[repoPath+"rebuilt/p"+forTest]
	gotest.NotZero(t, external)
	gotest.NotZero(t, variant)
	q := external.Imports[repoPath+"rebuilt/q"]
	gotest.NotZero(t, q)
	t.It("imports q rebuilt for the test", func(it *gotest.T) {
		gotest.Equal(it, repoPath+"rebuilt/q"+forTest, q.ID)
	})
	t.It("sees one p through q and in the test variant", func(it *gotest.T) {
		made := q.Types.Scope().Lookup("Make").Type().(*types.Signature).Results().At(0).Type()
		item := variant.Types.Scope().Lookup("Item").Type()
		gotest.True(it, types.Identical(item, made), "q.Make returns %s, the test variant declares %s", made, item)
	})
}
