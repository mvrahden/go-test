package gotestgen //nolint:stdlib-test

import (
	"embed"
	"fmt"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/packages"
)

//go:embed testdata/sources
var testSources embed.FS

var (
	testPkgIndex map[string]*packages.Package
	testPkgDir   string
	testPkgErr   error
)

func TestMain(m *testing.M) {
	testPkgIndex, testPkgDir, testPkgErr = loadAllTestPkgs()
	code := m.Run()
	if testPkgDir != "" {
		os.RemoveAll(testPkgDir)
	}
	os.Exit(code)
}

func loadAllTestPkgs() (map[string]*packages.Package, string, error) {
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return nil, "", err
	}

	scratch, err := os.MkdirTemp("", "gotest-mod-init-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(scratch)

	goMod := []byte("module testpkg\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + modRoot + "\n")
	if err := os.WriteFile(filepath.Join(scratch, "go.mod"), goMod, 0644); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(scratch, "stub.go"), []byte("package testpkg\n\nimport _ \"github.com/mvrahden/go-test/pkg/gotest\"\n"), 0644); err != nil {
		return nil, "", err
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	dir, err := os.MkdirTemp("", "gotest-batch-root-*")
	if err != nil {
		return nil, "", err
	}

	tidiedMod, _ := os.ReadFile(filepath.Join(scratch, "go.mod"))
	tidiedSum, _ := os.ReadFile(filepath.Join(scratch, "go.sum"))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), tidiedMod, 0644); err != nil {
		return nil, dir, err
	}
	if len(tidiedSum) > 0 {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), tidiedSum, 0644); err != nil {
			return nil, dir, err
		}
	}

	entries, err := fs.ReadDir(testSources, "testdata/sources")
	if err != nil {
		return nil, dir, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := fs.ReadFile(testSources, "testdata/sources/"+name+"/test.go")
		if err != nil {
			return nil, dir, err
		}
		subDir := filepath.Join(dir, name)
		if err := os.MkdirAll(subDir, 0755); err != nil {
			return nil, dir, err
		}
		if err := os.WriteFile(filepath.Join(subDir, "test.go"), data, 0644); err != nil {
			return nil, dir, err
		}
	}

	pkgs, err := packages.Load(&packages.Config{
		Mode: packageEvalMode,
		Dir:  dir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}, "./...")
	if err != nil {
		return nil, dir, err
	}

	index := make(map[string]*packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		parts := strings.Split(pkg.PkgPath, "/")
		key := parts[len(parts)-1]
		index[key] = pkg
	}
	return index, dir, nil
}

// ExportMustTestPkg looks up a test package by explicit name — used by pxtest suites.
func ExportMustTestPkg(t testing.TB, name string) *packages.Package {
	t.Helper()
	if testPkgErr != nil {
		t.Fatal(testPkgErr)
	}
	pkg, ok := testPkgIndex[name]
	if !ok {
		t.Fatalf("no test package found for %s", name)
	}
	if len(pkg.Errors) > 0 {
		t.Fatalf("package errors: %v", pkg.Errors)
	}
	return pkg
}

// Type aliases for unexported types used in tests across all gotestgen test files.
type ExportCollector = collector
type ExportRenderer = renderer

// Function exports for all gotestgen test files (Tasks 10-12).
var ExportPackageEvalMode = packageEvalMode
var ExportIsInternalPkgPath = isInternalPkgPath
var ExportBuildAllFixtureViewModels = buildAllFixtureViewModels

// ExportMakeFixtureSpec creates a minimal FixtureSpec for validation testing.
func ExportMakeFixtureSpec(name string, kind gotestast.FixtureKind, hasBeforeAll bool) *gotestast.FixtureSpec {
	f := gotestast.NewFixtureSpecForTest(name, kind)
	if hasBeforeAll {
		f.BeforeAll = types.NewFunc(token.NoPos, nil, "BeforeAll", nil)
	}
	return f
}
