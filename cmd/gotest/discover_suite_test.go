package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoverTestSuite covers the discover subcommand's JSON: suites, behaviors and benchmarks read from a staged fixture module.
//
//nolint:lifecycle-pair // BeforeAll only records the repo root; nothing is acquired
type DiscoverTestSuite struct{ repoRoot string }

func (s *DiscoverTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *DiscoverTestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot
}

func (s *DiscoverTestSuite) TestRunDiscover_SimpleSuite(t *gotest.T) {
	t.It("discovers suites in examples/cart", func(it *gotest.T) {
		absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
		gotest.NoError(it, err, "%v", err)
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.Skipf("examples directory not found: %v", err)
		}

		loadResults, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		gotest.NoError(it, err, "LoadPackages: %v", err)
		gotest.NotEmpty(it, loadResults, "expected at least one load result")

		out := ExportDiscoverOutput{}
		c := gotestgen.NewCollector()
		for _, lr := range loadResults {
			pkgEntry := ExportDiscoverPackage{
				ImportPath: lr.PkgPath,
				Dir:        lr.PkgDir,
			}

			collect := func(pkg *packages.Package) {
				result := c.CollectSuiteSpecs(pkg)
				var collectorErrs []string
				for _, cerr := range result.Errs {
					collectorErrs = append(collectorErrs, cerr.Err.Error())
				}
				gotest.Empty(it, collectorErrs, "collector errors")
				for _, suite := range result.Suites {
					pkgEntry.Suites = append(pkgEntry.Suites, ExportBuildDiscoverSuite(suite))
				}
			}
			if lr.Ptest != nil {
				collect(lr.Ptest)
			}
			if lr.Pxtest != nil {
				collect(lr.Pxtest)
			}

			out.Packages = append(out.Packages, pkgEntry)
		}

		gotest.Len(it, out.Packages, 1)

		pkg := out.Packages[0]
		gotest.Equal(it, "github.com/mvrahden/go-test/examples/cart", pkg.ImportPath)
		gotest.True(it, filepath.IsAbs(pkg.Dir), "dir should be absolute, got %q", pkg.Dir)

		gotest.Len(it, pkg.Suites, 4)

		suiteByNameAndFile := map[string]ExportDiscoverSuite{}
		for i := range pkg.Suites {
			suiteByNameAndFile[pkg.Suites[i].Name+":"+pkg.Suites[i].File] = pkg.Suites[i]
		}

		// Verify ptest ShoppingCartTestSuite
		st := suiteByNameAndFile["ShoppingCartTestSuite:suite_test.go"]
		gotest.Equal(it, "ShoppingCartTestSuite", st.Name)
		gotest.False(it, st.Parallel)
		gotest.False(it, st.Focused)
		gotest.False(it, st.Excluded)
		gotest.Equal(it, "suite_test.go", st.File)
		gotest.Equal(it, 5, st.Line)
		gotest.Equal(it, 6, st.Col)

		expectedLifecycle := []string{"BeforeEach"}
		gotest.Equal(it, expectedLifecycle, st.Lifecycle)
		gotest.Empty(it, st.Fixtures)

		gotest.Len(it, st.Methods, 9)
		gotest.Equal(it, "TestAddSingleItem", st.Methods[0].Name)
		gotest.Equal(it, 15, st.Methods[0].Line)
		gotest.Equal(it, 1, st.Methods[0].Col)
		gotest.Equal(it, "TestAddMultipleItems", st.Methods[1].Name)

		// Verify ptest PricingTestSuite (fixture-bound)
		pt := suiteByNameAndFile["PricingTestSuite:pricing_suite_test.go"]
		gotest.Equal(it, "PricingTestSuite", pt.Name)

		// Verify pxtest ShoppingCartTestSuite
		sx := suiteByNameAndFile["ShoppingCartTestSuite:suite_ext_test.go"]
		gotest.Equal(it, "ShoppingCartTestSuite", sx.Name)
		gotest.Len(it, sx.Methods, 2)
		gotest.Equal(it, "TestAddItem", sx.Methods[0].Name)
		gotest.Equal(it, "TestRemoveItem", sx.Methods[1].Name)

		// Verify pxtest PricingExtTestSuite (fixture-bound)
		px := suiteByNameAndFile["PricingExtTestSuite:pricing_ext_suite_test.go"]
		gotest.Equal(it, "PricingExtTestSuite", px.Name)

		// Verify JSON serialization roundtrip
		data, err := json.Marshal(out)
		gotest.NoError(it, err, "json.Marshal: %v", err)
		var roundtrip ExportDiscoverOutput
		gotest.NoError(it, json.Unmarshal(data, &roundtrip))
		gotest.Len(it, roundtrip.Packages, 1)
	})
}

func (s *DiscoverTestSuite) TestRunDiscover_Benchmarks(t *gotest.T) {
	t.It("includes benchmark methods in discover JSON, marking exclusions", func(it *gotest.T) {
		srcPath := filepath.Join(
			s.repoRoot, "internal", "gotestgen", "testdata", "sources",
			"TestCollector_BenchmarkMethod", "test.go",
		)
		src, err := os.ReadFile(srcPath)
		gotest.NoError(it, err)

		fixtureDir := stageFixtureModule(it, s.repoRoot, it.TempDir(), "bench_fixture.go", src)

		// gotestgen.LoadPackages requires Tests:true's "[pkg.test]" variant,
		// which only exists for packages with _test.go files; this fixture
		// (copied verbatim from the Task 3 testdata, filename "test.go") has
		// none, so load it as a plain package instead — CollectSuiteSpecs
		// only needs Syntax/Types, not a test-binary variant.
		pkgs, err := packages.Load(&packages.Config{
			Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
			Dir: fixtureDir,
		}, ".")
		gotest.NoError(it, err)
		gotest.Len(it, pkgs, 1)
		gotest.Empty(it, pkgs[0].Errors, "expected no package load errors, got: %v", pkgs[0].Errors)

		c := gotestgen.NewCollector()
		result := c.CollectSuiteSpecs(pkgs[0])
		gotest.Empty(it, result.Errs, "expected no collector errors, got: %v", result.Errs)
		gotest.Len(it, result.Suites, 1)

		ds := ExportBuildDiscoverSuite(result.Suites[0])
		data, err := json.Marshal(ds)
		gotest.NoError(it, err)
		payload := string(data)

		gotest.Contains(it, payload, `"benchmarks":[{"name":"BenchmarkParse"`)
		gotest.Contains(it, payload, `"X_BenchmarkOld"`)
		gotest.Contains(it, payload, `"excluded":true`)
	})

	t.It("includes fuzz methods in discover JSON, marking exclusions", func(it *gotest.T) {
		srcPath := filepath.Join(
			s.repoRoot, "internal", "gotestgen", "testdata", "sources",
			"TestCollector_FuzzMethod", "test.go",
		)
		src, err := os.ReadFile(srcPath)
		gotest.NoError(it, err)

		fixtureDir := stageFixtureModule(it, s.repoRoot, it.TempDir(), "fuzz_fixture.go", src)

		pkgs, err := packages.Load(&packages.Config{
			Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
			Dir: fixtureDir,
		}, ".")
		gotest.NoError(it, err)
		gotest.Len(it, pkgs, 1)
		gotest.Empty(it, pkgs[0].Errors, "expected no package load errors, got: %v", pkgs[0].Errors)

		c := gotestgen.NewCollector()
		result := c.CollectSuiteSpecs(pkgs[0])
		gotest.Empty(it, result.Errs, "expected no collector errors, got: %v", result.Errs)
		gotest.Len(it, result.Suites, 1)

		ds := ExportBuildDiscoverSuite(result.Suites[0])
		data, err := json.Marshal(ds)
		gotest.NoError(it, err)
		payload := string(data)

		gotest.Contains(it, payload, `"fuzzers":[{"name":"FuzzParse"`)
		gotest.Contains(it, payload, `"X_FuzzOld"`)
		gotest.Contains(it, payload, `"excluded":true`)
		// The plain test method must stay out of the fuzzers list.
		gotest.NotContains(it, payload, `"fuzzers":[{"name":"TestOne"`)
	})
}
