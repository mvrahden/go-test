package gotestgen

import (
	"go/token"

	"github.com/mvrahden/go-test/internal/gotestast"
)

type DiscoverOutput struct {
	Packages []DiscoverPackage `json:"packages"`
}

type DiscoverPackage struct {
	ImportPath string          `json:"importPath"`
	Dir        string          `json:"dir"`
	Suites     []DiscoverSuite `json:"suites"`
}

type DiscoverSuite struct {
	Name      string           `json:"name"`
	Parallel  bool             `json:"parallel"`
	Focused   bool             `json:"focused"`
	Excluded  bool             `json:"excluded"`
	File      string           `json:"file"`
	Line      int              `json:"line"`
	Col       int              `json:"col"`
	Lifecycle []string         `json:"lifecycle"`
	Fixtures  []string         `json:"fixtures"`
	Methods   []DiscoverMethod `json:"methods"`
}

type DiscoverMethod struct {
	Name     string `json:"name"`
	Parallel bool   `json:"parallel"`
	Focused  bool   `json:"focused"`
	Excluded bool   `json:"excluded"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
}

func Discover(targetPkgs []string) (*DiscoverOutput, error) {
	loadResults, err := LoadPackages(targetPkgs, nil)
	if err != nil {
		return nil, err
	}

	pkgMap := make(map[string]*DiscoverPackage)
	var pkgOrder []string

	c := collector{}
	for _, lr := range loadResults {
		ptestCollected := c.CollectSuiteSpecs(lr.Ptest)
		if len(ptestCollected.Errs) > 0 {
			return nil, ptestCollected.Errs[0].Err
		}
		pxtestCollected := c.CollectSuiteSpecs(lr.Pxtest)
		if len(pxtestCollected.Errs) > 0 {
			return nil, pxtestCollected.Errs[0].Err
		}

		pkg, ok := pkgMap[lr.PkgPath]
		if !ok {
			pkg = &DiscoverPackage{
				ImportPath: lr.PkgPath,
				Dir:        lr.PkgDir,
			}
			pkgMap[lr.PkgPath] = pkg
			pkgOrder = append(pkgOrder, lr.PkgPath)
		}

		addSuites(pkg, ptestCollected.Suites)
		addSuites(pkg, pxtestCollected.Suites)
	}

	output := &DiscoverOutput{Packages: make([]DiscoverPackage, 0)}
	for _, path := range pkgOrder {
		p := pkgMap[path]
		if len(p.Suites) > 0 {
			output.Packages = append(output.Packages, *p)
		}
	}
	return output, nil
}

func addSuites(pkg *DiscoverPackage, suites gotestast.TestSuiteSpecSet) {
	for _, suite := range suites {
		pkg.Suites = append(pkg.Suites, convertSuite(suite))
	}
}

func convertSuite(suite *gotestast.TestSuiteSpec) DiscoverSuite {
	fset := suite.Package().Fset
	pos := fset.Position(suite.TypeSpecPos())

	lifecycle := collectLifecycle(suite)
	fixtures := make([]string, 0)
	if f := suite.Fixture(); f != nil {
		fixtures = append(fixtures, f.Identifier())
	}
	methods := make([]DiscoverMethod, 0, len(suite.TestCases()))
	for _, tc := range suite.TestCases() {
		methods = append(methods, convertMethod(tc, fset))
	}

	ds := DiscoverSuite{
		Name:      suite.Identifier(),
		Parallel:  suite.IsMethodParallel(),
		Focused:   suite.IsFocused(),
		Excluded:  suite.IsExcluded(),
		File:      pos.Filename,
		Line:      pos.Line,
		Col:       pos.Column,
		Lifecycle: lifecycle,
		Fixtures:  fixtures,
		Methods:   methods,
	}

	return ds
}

func collectLifecycle(suite *gotestast.TestSuiteSpec) []string {
	lc := make([]string, 0)
	if suite.BeforeAll() != nil {
		lc = append(lc, "BeforeAll")
	}
	if suite.AfterAll() != nil {
		lc = append(lc, "AfterAll")
	}
	if suite.BeforeEach() != nil {
		lc = append(lc, "BeforeEach")
	}
	if suite.AfterEach() != nil {
		lc = append(lc, "AfterEach")
	}
	return lc
}

func convertMethod(m *gotestast.TestSuiteMethod, fset *token.FileSet) DiscoverMethod {
	pos := fset.Position(m.Pos())
	return DiscoverMethod{
		Name:     m.Identifier(),
		Parallel: false,
		Focused:  m.IsFocused(),
		Excluded: m.IsExcluded(),
		File:     pos.Filename,
		Line:     pos.Line,
		Col:      pos.Column,
	}
}
