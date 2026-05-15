package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

// discoverOutput is the top-level JSON structure emitted by "gotest discover".
type discoverOutput struct {
	Packages []discoverPackage  `json:"packages"`
	Warnings []discoverWarning  `json:"warnings,omitempty"`
}

type discoverWarning struct {
	ImportPath string `json:"importPath"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Col        int    `json:"col,omitempty"`
	Message    string `json:"message"`
}

type discoverPackage struct {
	ImportPath string          `json:"importPath"`
	Dir        string          `json:"dir"`
	TestOnly   bool            `json:"testOnly,omitempty"`
	Suites     []discoverSuite `json:"suites"`
}

type discoverSuite struct {
	Name      string           `json:"name"`
	Parallel  bool             `json:"parallel"`
	Focused   bool             `json:"focused"`
	Excluded  bool             `json:"excluded"`
	Guarded   bool             `json:"guarded"`
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

	loadResults, loadWarnings, err := gotestgen.LoadPackagesForDiscovery(patterns, buildFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	for _, w := range loadWarnings {
		out.Warnings = append(out.Warnings, discoverWarning{
			ImportPath: w.PkgPath,
			Message:    w.Message,
		})
	}

	c := gotestgen.NewCollector()

	for _, lr := range loadResults {
		pkgEntry := discoverPackage{
			ImportPath: lr.PkgPath,
			Dir:        lr.PkgDir,
			TestOnly:   lr.IsTestOnly(),
		}

		hasWarnings := false
		pkgs := []*packages.Package{lr.Ptest, lr.Pxtest}
		for _, pkg := range pkgs {
			if pkg == nil {
				continue
			}
			result := c.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				hasWarnings = true
				for _, ce := range result.Errs {
					w := discoverWarning{
						ImportPath: lr.PkgPath,
						Message:    ce.Err.Error(),
					}
					if ce.Pos.IsValid() {
						pos := pkg.Fset.Position(ce.Pos)
						w.File = filepath.Base(pos.Filename)
						w.Line = pos.Line
						w.Col = pos.Column
					}
					out.Warnings = append(out.Warnings, w)
				}
				continue
			}

			for _, suite := range result.Suites {
				ds := buildDiscoverSuite(suite)
				ds.Fixtures = discoverFixtureNames(suite)
				pkgEntry.Suites = append(pkgEntry.Suites, ds)
			}
		}

		if len(pkgEntry.Suites) > 0 || hasWarnings {
			out.Packages = append(out.Packages, pkgEntry)
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
		Parallel: suite.IsMethodParallel(),
		Focused:  suite.IsFocused(),
		Excluded: suite.IsExcluded(),
		Guarded:  suite.HasGuard(),
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

	ds.Fixtures = []string{}

	// Methods (test cases)
	var methods []discoverMethod
	for _, tc := range suite.TestCases() {
		mPos := fset.Position(tc.Pos())
		methods = append(methods, discoverMethod{
			Name:     tc.Identifier(),
			Parallel: false,
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

func discoverFixtureNames(suite *gotestast.TestSuiteSpec) []string {
	st := suite.StructType()
	if st == nil {
		return []string{}
	}
	var names []string
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		ptr, ok := field.Type().(*types.Pointer)
		if !ok {
			continue
		}
		named, ok := ptr.Elem().(*types.Named)
		if !ok {
			continue
		}
		name := named.Obj().Name()
		if strings.HasSuffix(name, "Fixture") && !strings.HasSuffix(name, "SharedFixture") {
			names = append(names, name)
		}
	}
	if names == nil {
		return []string{}
	}
	return names
}
