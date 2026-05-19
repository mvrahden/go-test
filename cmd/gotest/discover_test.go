package main //nolint:stdlib-test

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

	loadResults, err := gotestgen.LoadPackages([]string{"./cart"}, nil)
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
	if pkg.ImportPath != "github.com/mvrahden/go-test/examples/cart" {
		t.Errorf("importPath = %q, want github.com/mvrahden/go-test/examples/cart", pkg.ImportPath)
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
	if s.Name != "ShoppingCartTestSuite" {
		t.Errorf("suite name = %q, want ShoppingCartTestSuite", s.Name)
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
	if s.File != "suite_test.go" {
		t.Errorf("file = %q, want suite_test.go", s.File)
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
	if len(s.Methods) != 9 {
		t.Fatalf("expected 9 methods, got %d", len(s.Methods))
	}
	if s.Methods[0].Name != "TestAddSingleItem" {
		t.Errorf("method[0] name = %q, want TestAddSingleItem", s.Methods[0].Name)
	}
	if s.Methods[0].Line != 15 {
		t.Errorf("method[0] line = %d, want 15", s.Methods[0].Line)
	}
	if s.Methods[0].Col != 1 {
		t.Errorf("method[0] col = %d, want 1", s.Methods[0].Col)
	}
	if s.Methods[1].Name != "TestAddMultipleItems" {
		t.Errorf("method[1] name = %q, want TestAddMultipleItems", s.Methods[1].Name)
	}

	// Verify the pxtest suite
	sx := pkg.Suites[1]
	if sx.Name != "ShoppingCartTestSuite" {
		t.Errorf("pxtest suite name = %q, want ShoppingCartTestSuite", sx.Name)
	}
	if len(sx.Methods) != 2 {
		t.Fatalf("expected 2 pxtest methods, got %d", len(sx.Methods))
	}
	if sx.Methods[0].Name != "TestAddItem" {
		t.Errorf("pxtest method[0] name = %q, want TestAddItem", sx.Methods[0].Name)
	}
	if sx.Methods[1].Name != "TestRemoveItem" {
		t.Errorf("pxtest method[1] name = %q, want TestRemoveItem", sx.Methods[1].Name)
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
