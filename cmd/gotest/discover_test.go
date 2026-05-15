package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func TestRunDiscover_SimpleSuite(t *testing.T) {
	// We need to run discover from the examples directory since it's a separate module.
	// Instead, use the underlying discover logic directly (LoadPackages + collector).
	examplesDir := filepath.Join("..", "..", "examples")
	if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
		t.Skipf("examples directory not found: %v", err)
	}

	// Change to examples dir to load packages in that module context
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absExamples, err := filepath.Abs(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(absExamples); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	loadResults, err := gotestgen.LoadPackages([]string{"./simple_suite"}, nil)
	if err != nil {
		t.Fatalf("LoadPackages: %v", err)
	}
	if len(loadResults) == 0 {
		t.Fatal("expected at least one load result")
	}

	// Build discover output using the same logic as runDiscover
	out := discoverOutput{}
	c := gotestgen.NewCollector()
	for _, lr := range loadResults {
		pkgEntry := discoverPackage{
			ImportPath: lr.PkgPath,
			Dir:        lr.PkgDir,
		}

		// ptest
		if lr.Ptest != nil {
			result := c.CollectSuiteSpecs(lr.Ptest)
			if len(result.Errs) > 0 {
				t.Fatalf("collector error: %v", result.Errs[0].Err)
			}
			for _, suite := range result.Suites {
				pkgEntry.Suites = append(pkgEntry.Suites, buildDiscoverSuite(suite))
			}
		}
		// pxtest
		if lr.Pxtest != nil {
			result := c.CollectSuiteSpecs(lr.Pxtest)
			if len(result.Errs) > 0 {
				t.Fatalf("collector error: %v", result.Errs[0].Err)
			}
			for _, suite := range result.Suites {
				pkgEntry.Suites = append(pkgEntry.Suites, buildDiscoverSuite(suite))
			}
		}

		out.Packages = append(out.Packages, pkgEntry)
	}

	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}

	pkg := out.Packages[0]
	if pkg.ImportPath != "github.com/mvrahden/go-test/examples/simple_suite" {
		t.Errorf("importPath = %q, want github.com/mvrahden/go-test/examples/simple_suite", pkg.ImportPath)
	}
	if !filepath.IsAbs(pkg.Dir) {
		t.Errorf("dir should be absolute, got %q", pkg.Dir)
	}

	// Should have 2 suites: one from ptest, one from pxtest
	if len(pkg.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(pkg.Suites))
	}

	// Verify the ptest suite
	s := pkg.Suites[0]
	if s.Name != "SimpleTestSuite" {
		t.Errorf("suite name = %q, want SimpleTestSuite", s.Name)
	}
	if s.Parallel {
		t.Error("suite should not be parallel")
	}
	if s.Focused {
		t.Error("suite should not be focused")
	}
	if s.Excluded {
		t.Error("suite should not be excluded")
	}
	if s.File != "ptest_test.go" {
		t.Errorf("file = %q, want ptest_test.go", s.File)
	}
	if s.Line != 5 {
		t.Errorf("line = %d, want 5", s.Line)
	}
	if s.Col != 6 {
		t.Errorf("col = %d, want 6", s.Col)
	}

	// Lifecycle hooks
	expectedLifecycle := []string{"BeforeEach"}
	if len(s.Lifecycle) != len(expectedLifecycle) {
		t.Errorf("lifecycle = %v, want %v", s.Lifecycle, expectedLifecycle)
	} else {
		for i, lc := range s.Lifecycle {
			if lc != expectedLifecycle[i] {
				t.Errorf("lifecycle[%d] = %q, want %q", i, lc, expectedLifecycle[i])
			}
		}
	}

	// Fixtures
	if len(s.Fixtures) != 0 {
		t.Errorf("fixtures = %v, want empty", s.Fixtures)
	}

	// Methods
	if len(s.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(s.Methods))
	}
	if s.Methods[0].Name != "TestLength" {
		t.Errorf("method[0] name = %q, want TestLength", s.Methods[0].Name)
	}
	if s.Methods[0].Line != 13 {
		t.Errorf("method[0] line = %d, want 13", s.Methods[0].Line)
	}
	if s.Methods[0].Col != 1 {
		t.Errorf("method[0] col = %d, want 1", s.Methods[0].Col)
	}
	if s.Methods[1].Name != "TestContains" {
		t.Errorf("method[1] name = %q, want TestContains", s.Methods[1].Name)
	}

	// Verify JSON serialization roundtrip
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var roundtrip discoverOutput
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(roundtrip.Packages) != 1 {
		t.Fatalf("roundtrip: expected 1 package, got %d", len(roundtrip.Packages))
	}
}

func TestRunDiscover_FocusExclude(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
		t.Skipf("examples directory not found: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absExamples, err := filepath.Abs(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(absExamples); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	loadResults, err := gotestgen.LoadPackages([]string{"./focus_exclude"}, nil)
	if err != nil {
		t.Fatalf("LoadPackages: %v", err)
	}

	out := discoverOutput{}
	c := gotestgen.NewCollector()
	for _, lr := range loadResults {
		pkgEntry := discoverPackage{
			ImportPath: lr.PkgPath,
			Dir:        lr.PkgDir,
		}
		if lr.Ptest != nil {
			result := c.CollectSuiteSpecs(lr.Ptest)
			if len(result.Errs) > 0 {
				t.Fatalf("collector error: %v", result.Errs[0].Err)
			}
			for _, suite := range result.Suites {
				pkgEntry.Suites = append(pkgEntry.Suites, buildDiscoverSuite(suite))
			}
		}
		if lr.Pxtest != nil {
			result := c.CollectSuiteSpecs(lr.Pxtest)
			if len(result.Errs) > 0 {
				t.Fatalf("collector error: %v", result.Errs[0].Err)
			}
			for _, suite := range result.Suites {
				pkgEntry.Suites = append(pkgEntry.Suites, buildDiscoverSuite(suite))
			}
		}
		out.Packages = append(out.Packages, pkgEntry)
	}

	if len(out.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(out.Packages))
	}

	// Find the focused suite from ptest
	var focused *discoverSuite
	var excluded *discoverSuite
	for i := range out.Packages[0].Suites {
		s := &out.Packages[0].Suites[i]
		if s.Name == "F_FocusedTestSuite" {
			focused = s
		}
		if s.Name == "X_ExcludedTestSuite" {
			excluded = s
		}
	}

	if focused == nil {
		t.Fatal("F_FocusedTestSuite not found")
	}
	if !focused.Focused {
		t.Error("F_FocusedTestSuite should be focused")
	}
	if focused.Excluded {
		t.Error("F_FocusedTestSuite should not be excluded")
	}

	// Check excluded method within focused suite
	var excludedMethod *discoverMethod
	for i := range focused.Methods {
		if focused.Methods[i].Name == "X_TestGamma" {
			excludedMethod = &focused.Methods[i]
		}
	}
	if excludedMethod == nil {
		t.Fatal("X_TestGamma not found in F_FocusedTestSuite")
	}
	if !excludedMethod.Excluded {
		t.Error("X_TestGamma should be excluded")
	}

	if excluded == nil {
		t.Fatal("X_ExcludedTestSuite not found")
	}
	if !excluded.Excluded {
		t.Error("X_ExcludedTestSuite should be excluded")
	}
	if excluded.Focused {
		t.Error("X_ExcludedTestSuite should not be focused")
	}
}
