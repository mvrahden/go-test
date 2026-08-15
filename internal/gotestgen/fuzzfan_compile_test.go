package gotestgen_test //nolint:stdlib-test // shells out to the go tool; no assertions to express as a suite

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"golang.org/x/tools/go/packages"
)

// TestGeneratedFuzzFans_CompileAndRoundTrip writes the ACTUAL emitted fan
// source into an isolated module beside the type definitions it fans and a
// hand-written property test that drives the generated register function
// through a real (*testing.F).Fuzz, then runs `go test`. Substring
// assertions cannot catch a non-compiling emitter, an emitted call to a
// leaf helper that does not exist, or a hybrid decoder that panics on a
// short input — this can.
//
// It also validates the literal functions in two phases, since a literal is
// a runtime-computed STRING that itself has to be compiled to check it:
// phase 1 calls Rich's literal function for a handful of representative
// values and prints the resulting Go source text (this alone already
// exercises "does fan_gen.go compile" — the exact failure mode a
// substring assertion cannot catch; this branch shipped that bug once, in
// scaffold --fuzz); phase 2 splices each printed literal in as a map value
// initialiser in a companion file and compiles THAT, then compares the
// reconstructed value against the original with reflect.DeepEqual.
//
// Mirrors the module+replace pattern in internal/scaffold/fuzz_test.go and
// internal/gotestgen/export_test.go.
func TestGeneratedFuzzFans_CompileAndRoundTrip(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	set := buildFuzzfanFixtureSet(t)
	if set == nil || len(set.Fans) == 0 {
		t.Fatal("expected a fan for testdata/fuzzfan, got none")
	}

	// Rich — which exercises every literal-supported shape — must carry a
	// literal function; the register function is what the check test drives.
	litFunc := "ƒ_fuzzlits_v1_Rich"
	if !strings.Contains(set.Fans[0].Expr, "Literal: "+litFunc+"}") {
		t.Fatalf("expected Rich's fan to reference %s, got %s", litFunc, set.Fans[0].Expr)
	}

	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	write("go.mod", []byte("module fuzzfancheck\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => "+repoRoot+"\n"))

	fixtureTypes, err := os.ReadFile(filepath.Join("testdata", "fuzzfan", "types.go"))
	if err != nil {
		t.Fatalf("reading fixture types: %v", err)
	}
	write("types.go", fixtureTypes)

	check, err := os.ReadFile(filepath.Join("testdata", "fuzzfan", "check", "roundtrip_test.go"))
	if err != nil {
		t.Fatalf("reading check test: %v", err)
	}
	write("roundtrip_test.go", check)

	imports := []string{`"testing"`, `"github.com/mvrahden/go-test/pkg/gotestfuzz"`}
	if set.NeedsStrings {
		imports = append(imports, `"strings"`)
	}
	if set.NeedsStrconv {
		imports = append(imports, `"strconv"`)
	}
	if set.NeedsMath {
		imports = append(imports, `"math"`)
	}
	write("fan_gen.go", []byte(
		"package fuzzfan\n\nimport (\n\t"+strings.Join(imports, "\n\t")+"\n)\n"+set.Source+fanExprsDecl(set)))

	// Phase 1: print each case's literal rendering to stdout, tab-separated
	// so it survives -v's output unambiguously.
	write("literal_print_test.go", []byte(literalPrintTestSrc(litFunc)))

	printCmd := exec.Command("go", "test", "-run", "TestPrintFuzzLiterals", "-v", "./...")
	printCmd.Dir = dir
	printCmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	printOut, err := printCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("literal functions failed to compile or run:\n%s\n--- generated source ---\n%s", printOut, set.Source)
	}

	cases, err := parseLiteralCases(printOut)
	if err != nil {
		t.Fatalf("parsing printed literals: %v\noutput:\n%s", err, printOut)
	}
	if len(cases) == 0 {
		t.Fatalf("no literal cases were printed\noutput:\n%s", printOut)
	}

	// Phase 2: splice every printed literal in as a map-value initialiser
	// and compile it — this is the reconstruction half the brief calls for:
	// parseable Go that actually rebuilds the value, checked with
	// reflect.DeepEqual (the NaN case gets a bit-pattern check instead,
	// since NaN != NaN under DeepEqual).
	write("literal_check_test.go", []byte(literalCheckTestSrc(cases)))

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated fans failed to compile or round-trip:\n%s\n--- generated source ---\n%s", out, set.Source)
	}
}

// fanExprsDecl renders every fan adapter expression the wrapper would attach
// to NewF as a package-level declaration, so the scratch module type-checks
// the expressions against the generated functions — exactly what the real
// generated file does — and never trips "imported and not used" for a fan
// whose functions happen not to touch the runtime package.
func fanExprsDecl(set *gotestgen.FuzzFanSet) string {
	var b strings.Builder
	b.WriteString("\nvar _ = []gotestfuzz.Adapter{\n")
	for _, f := range set.Fans {
		b.WriteString("\t")
		b.WriteString(f.Expr)
		b.WriteString(",\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// literalPrintTestSrc is phase 1's companion file: it calls litFunc — Rich's
// emitted literal function — over richCases() plus a NaN and an Inf case
// neither present there, and prints each result tagged by name.
func literalPrintTestSrc(litFunc string) string {
	return `package fuzzfan

import (
	"fmt"
	"testing"
)

func TestPrintFuzzLiterals(t *testing.T) {
	for i, v := range richCases() {
		fmt.Printf("LITCASE\t%d\t%s\n", i, ` + litFunc + `(v))
	}
}
`
}

// parseLiteralCases extracts the name -> literal-text pairs phase 1 printed.
// SplitN caps at 3 fields, so a tab inside the literal text itself (there
// never is one — strconv.Quote escapes real tabs — but this stays correct
// either way) cannot corrupt the split.
func parseLiteralCases(output []byte) (map[string]string, error) {
	cases := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "LITCASE\t") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("malformed LITCASE line: %q", line)
		}
		cases[parts[1]] = parts[2]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

// literalCheckTestSrc is phase 2's companion file: cases' values are the
// RAW LITERAL TEXT the generated function produced, spliced in verbatim as
// map-value initialisers, so this only compiles if that text is valid,
// self-contained Go.
func literalCheckTestSrc(cases map[string]string) string {
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("package fuzzfan\n\nimport (\n\t\"fmt\"\n\t\"math\"\n\t\"testing\"\n)\n\nvar _ = math.NaN\n\n")
	b.WriteString("var reconstructedByIndex = map[string]Rich{\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\t%q: %s,\n", name, cases[name])
	}
	b.WriteString("}\n\n")
	b.WriteString("func TestFuzzLiteralReconstruction(t *testing.T) {\n")
	b.WriteString("\tfor i, w := range richCases() {\n")
	b.WriteString("\t\tgot, ok := reconstructedByIndex[fmt.Sprint(i)]\n")
	b.WriteString("\t\tif !ok {\n\t\t\tt.Fatalf(\"missing reconstructed case %d\", i)\n\t\t\tcontinue\n\t\t}\n")
	b.WriteString("\t\tif !equalRich(w, got) {\n")
	b.WriteString("\t\t\tt.Fatalf(\"literal did not reconstruct the value for case %d:\\n got: %#v\\nwant: %#v\", i, got, w)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// buildFuzzfanFixtureSet loads testdata/fuzzfan with Tests: true (so the
// suite in its _test.go file is visible) and emits its fans.
func buildFuzzfanFixtureSet(t *testing.T) *gotestgen.FuzzFanSet {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, "./testdata/fuzzfan")
	if err != nil {
		t.Fatalf("loading testdata/fuzzfan: %v", err)
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
		set, err := gotestgen.BuildFuzzFans(p, spec.EffectiveTestSuites)
		if err != nil {
			t.Fatalf("BuildFuzzFans: %v", err)
		}
		return set
	}
	t.Fatal("expected the ptest package variant for testdata/fuzzfan")
	return nil
}

// TestCrossPackageLiteral_Compiles proves the qualifier fix for a Critical
// review finding actually compiles: literalBasicWrapped once wrapped a
// named basic with its bare identifier (named.Obj().Name()) rather than the
// qualified type expression (e.typeRef(t)), so a cross-package named basic
// like crossdep.ID rendered as the literal "ID(...)" — a bare identifier
// that is out of scope outside the package that declares it, and so fails
// to compile wherever the literal gets spliced. The unit-test assertions in
// TestCrossPackageTypes catch the wrong TEXT; this proves the right text
// actually compiles, by writing the crossdep package into a scratch module
// (module path "testpkg", subdirectory "TestFuzzCodec_CrossDep" — matching
// the import path BuildFuzzFans recorded for the real fixture package) and
// building the real emitted source against it.
func TestCrossPackageLiteral_Compiles(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	pkg := gotestgen.ExportMustTestPkg(t, "TestFuzzCodec_CrossPackageBasic")
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	if len(result.Errs) > 0 {
		t.Fatalf("collection errors: %v", result.Errs)
	}
	spec, err := c.ApplyTestSuiteSpecs(result)
	if err != nil {
		t.Fatalf("ApplyTestSuiteSpecs: %v", err)
	}
	set, err := gotestgen.BuildFuzzFans(pkg, spec.EffectiveTestSuites)
	if err != nil {
		t.Fatalf("BuildFuzzFans: %v", err)
	}

	if !strings.Contains(set.Source, `"crossdep.ID("`) {
		t.Fatalf("expected the literal to reference the qualified crossdep.ID, got:\n%s", set.Source)
	}
	if strings.Contains(set.Source, `"ID("`) {
		t.Fatalf("literal rendered the bare, out-of-scope identifier:\n%s", set.Source)
	}

	dir := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, data, 0600); err != nil {
			t.Fatalf("writing %s: %v", rel, err)
		}
	}

	write("go.mod", []byte("module testpkg\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => "+repoRoot+"\n"))

	crossdepSrc, err := os.ReadFile(filepath.Join("testdata", "sources", "TestFuzzCodec_CrossDep", "test.go"))
	if err != nil {
		t.Fatalf("reading crossdep fixture: %v", err)
	}
	write("TestFuzzCodec_CrossDep/test.go", crossdepSrc)

	imports := []string{`"testing"`, `"github.com/mvrahden/go-test/pkg/gotestfuzz"`}
	if set.NeedsStrings {
		imports = append(imports, `"strings"`)
	}
	if set.NeedsStrconv {
		imports = append(imports, `"strconv"`)
	}
	if set.NeedsMath {
		imports = append(imports, `"math"`)
	}
	for _, p := range set.PkgPaths {
		imports = append(imports, fmt.Sprintf("%q", p))
	}
	write("fan_gen.go", []byte(
		"package check\n\nimport (\n\t"+strings.Join(imports, "\n\t")+"\n)\n"+set.Source+fanExprsDecl(set)))

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated cross-package fan failed to compile:\n%s\n--- generated source ---\n%s", out, set.Source)
	}
}
