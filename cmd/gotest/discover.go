package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

// discoverOutput is the top-level JSON structure emitted by "gotest discover".
type discoverOutput struct {
	Packages []discoverPackage `json:"packages"`
}

type discoverPackage struct {
	ImportPath string          `json:"importPath"`
	Dir        string          `json:"dir"`
	Suites     []discoverSuite `json:"suites"`
}

type discoverSuite struct {
	Name      string           `json:"name"`
	Parallel  bool             `json:"parallel"`
	Focused   bool             `json:"focused"`
	Excluded  bool             `json:"excluded"`
	File      string           `json:"file"`
	Line      int              `json:"line"`
	Col       int              `json:"col"`
	Lifecycle []string         `json:"lifecycle"`
	Fixtures  []string         `json:"fixtures"`
	Methods   []discoverMethod `json:"methods"`
}

type discoverMethod struct {
	Name     string `json:"name"`
	Parallel bool   `json:"parallel"`
	Focused  bool   `json:"focused"`
	Excluded bool   `json:"excluded"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
}

func runDiscover(args []string) int {
	tags, remaining := extractTagsFlag(args)
	patterns := ExtractPackagePatterns(remaining)
	var buildFlags []string
	if tags != "" {
		buildFlags = append(buildFlags, "-tags="+tags)
	}

	out := discoverOutput{}

	for _, pattern := range patterns {
		loadResults, err := gotestgen.LoadPackages(pattern, buildFlags...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}

		c := gotestgen.NewCollector()

		for _, lr := range loadResults {
			pkgEntry := discoverPackage{
				ImportPath: lr.PkgPath,
				Dir:        lr.PkgDir,
			}

			pkgs := []*packages.Package{lr.Ptest, lr.Pxtest}
			for _, pkg := range pkgs {
				if pkg == nil {
					continue
				}
				result := c.CollectSuiteSpecs(pkg)
				if len(result.Errs) > 0 {
					fmt.Fprintf(os.Stderr, "FAIL: %s\n", result.Errs[0].Err)
					return 2
				}

				if _, err := gotestgen.Resolve(pkg, result.Suites, result.Fixtures); err != nil {
					fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
					return 2
				}

				for _, suite := range result.Suites {
					ds := buildDiscoverSuite(suite)
					pkgEntry.Suites = append(pkgEntry.Suites, ds)
				}
			}

			if len(pkgEntry.Suites) > 0 {
				out.Packages = append(out.Packages, pkgEntry)
			}
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return 0
}

func buildDiscoverSuite(suite *gotestast.TestSuiteSpec) discoverSuite {
	pkg := suite.Package()
	fset := pkg.Fset
	pos := fset.Position(suite.TypeSpecPos())

	ds := discoverSuite{
		Name:     suite.Identifier(),
		Parallel: suite.IsParallelSuite(),
		Focused:  suite.IsFocused(),
		Excluded: suite.IsExcluded(),
		File:     filepath.Base(pos.Filename),
		Line:     pos.Line,
		Col:      pos.Column,
	}

	// Lifecycle hooks
	var lifecycle []string
	if suite.BeforeAll() != nil {
		lifecycle = append(lifecycle, "BeforeAll")
	}
	if suite.AfterAll() != nil {
		lifecycle = append(lifecycle, "AfterAll")
	}
	if suite.BeforeEach() != nil {
		lifecycle = append(lifecycle, "BeforeEach")
	}
	if suite.AfterEach() != nil {
		lifecycle = append(lifecycle, "AfterEach")
	}
	if lifecycle == nil {
		lifecycle = []string{}
	}
	ds.Lifecycle = lifecycle

	// Fixtures
	var fixtures []string
	if f := suite.Fixture(); f != nil {
		fixtures = append(fixtures, f.Identifier())
	}
	if fixtures == nil {
		fixtures = []string{}
	}
	ds.Fixtures = fixtures

	// Methods (test cases)
	var methods []discoverMethod
	for _, tc := range suite.TestCases() {
		mPos := fset.Position(tc.Pos())
		methods = append(methods, discoverMethod{
			Name:     tc.Identifier(),
			Parallel: tc.IsParallel(),
			Focused:  tc.IsFocused(),
			Excluded: tc.IsExcluded(),
			File:     filepath.Base(mPos.Filename),
			Line:     mPos.Line,
			Col:      mPos.Column,
		})
	}
	if methods == nil {
		methods = []discoverMethod{}
	}
	ds.Methods = methods

	return ds
}
