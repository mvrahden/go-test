package gotestgen_test //nolint:stdlib-test // shells out to the go tool; no assertions to express as a suite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"golang.org/x/tools/go/packages"
)

// TestGeneratedFuzzCodecs_CompileAndRoundTrip writes the ACTUAL emitted
// codec source into an isolated module beside the type definitions it
// decodes and a hand-written property test, then runs `go test`. Substring
// assertions cannot catch a non-compiling emitter, an emitted call to a
// FuzzReader method that does not exist, or a decoder that panics on a
// short input — this can.
//
// Mirrors the module+replace pattern in internal/scaffold/fuzz_test.go and
// internal/gotestgen/export_test.go.
func TestGeneratedFuzzCodecs_CompileAndRoundTrip(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	set := buildFuzzcodecFixtureSet(t)
	if set == nil {
		t.Fatal("expected codecs for testdata/fuzzcodec, got none")
	}

	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	write("go.mod", []byte("module fuzzcodeccheck\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => "+repoRoot+"\n"))

	fixtureTypes, err := os.ReadFile(filepath.Join("testdata", "fuzzcodec", "types.go"))
	if err != nil {
		t.Fatalf("reading fixture types: %v", err)
	}
	write("types.go", fixtureTypes)

	check, err := os.ReadFile(filepath.Join("testdata", "fuzzcodec", "check", "roundtrip_test.go"))
	if err != nil {
		t.Fatalf("reading check test: %v", err)
	}
	write("roundtrip_test.go", check)

	write("codec_gen.go", []byte(
		"package fuzzcodec\n\nimport \"github.com/mvrahden/go-test/pkg/gotestruntime\"\n"+set.Source))

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated codecs failed to compile or round-trip:\n%s\n--- generated source ---\n%s", out, set.Source)
	}
}

// buildFuzzcodecFixtureSet loads testdata/fuzzcodec with Tests: true (so the
// suite in its _test.go file is visible) and emits its codecs.
func buildFuzzcodecFixtureSet(t *testing.T) *gotestgen.FuzzCodecSet {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, "./testdata/fuzzcodec")
	if err != nil {
		t.Fatalf("loading testdata/fuzzcodec: %v", err)
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			t.Fatalf("package load errors for %s: %v", p.ID, p.Errors)
		}
	}
	for _, p := range pkgs {
		if !strings.HasSuffix(p.ID, ".test]") || strings.HasSuffix(p.Name, "_test") {
			continue
		}
		c := gotestgen.NewCollector()
		result := c.CollectSuiteSpecs(p)
		if len(result.Errs) > 0 {
			t.Fatalf("collection errors: %v", result.Errs)
		}
		spec, err := c.ApplyTestSuiteSpecs(result)
		if err != nil {
			t.Fatalf("ApplyTestSuiteSpecs: %v", err)
		}
		set, err := gotestgen.BuildFuzzCodecs(p, spec.EffectiveTestSuites)
		if err != nil {
			t.Fatalf("BuildFuzzCodecs: %v", err)
		}
		return set
	}
	t.Fatal("expected the ptest package variant for testdata/fuzzcodec")
	return nil
}
