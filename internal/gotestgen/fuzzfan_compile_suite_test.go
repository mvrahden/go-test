package gotestgen_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// FuzzFanCompileTestSuite builds the ACTUAL emitted fan source in a scratch
// module and drives it through the go tool. Substring assertions cannot
// catch a non-compiling emitter, a call to a leaf helper that does not
// exist, or a decoder that panics on a short input; this can.
type FuzzFanCompileTestSuite struct{}

func (s *FuzzFanCompileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	cfg.Timeout = 0
	return cfg
}

type fanCompileCtx struct {
	repoRoot string
	dir      string
}

func (s *FuzzFanCompileTestSuite) BeforeEach(t *gotest.T) *fanCompileCtx {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	return &fanCompileCtx{repoRoot: repoRoot, dir: t.TempDir()}
}

func (s *FuzzFanCompileTestSuite) write(t *gotest.T, ctx *fanCompileCtx, rel string, data []byte) {
	full := filepath.Join(ctx.dir, rel)
	gotest.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
	gotest.NoError(t, os.WriteFile(full, data, 0600))
}

func (s *FuzzFanCompileTestSuite) goMod(ctx *fanCompileCtx, module string) []byte {
	return []byte("module " + module + "\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + ctx.repoRoot + "\n")
}

// fanImports lists the imports the emitted source needs, as the wrapper's
// generated file declares them.
func fanImports(set *gotestgen.FuzzTargetSet) []string {
	imports := []string{`"testing"`, `"github.com/mvrahden/go-test/pkg/gotest"`}
	if set.NeedsRuntime {
		imports = append(imports, `"github.com/mvrahden/go-test/pkg/gotestfuzz"`)
	}
	if set.NeedsStrings {
		imports = append(imports, `"strings"`)
	}
	if set.NeedsStrconv {
		imports = append(imports, `"strconv"`)
	}
	if set.NeedsMath {
		imports = append(imports, `"math"`)
	}
	return imports
}

func (s *FuzzFanCompileTestSuite) run(t *gotest.T, ctx *fanCompileCtx, set *gotestgen.FuzzTargetSet, args ...string) []byte {
	cmd := exec.Command("go", args...)
	cmd.Dir = ctx.dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go %s:\n%s\n--- generated source ---\n%s", strings.Join(args, " "), out, set.Source)
	return out
}

// TestGeneratedFans drives the register function through a real
// (*testing.F).Fuzz and validates the literal functions in two phases: phase
// 1 prints the literal text Rich's function renders for representative
// values (proving fan_gen.go compiles, the bug scaffold --fuzz once
// shipped); phase 2 splices every printed literal in as a map value and
// compiles THAT, comparing the rebuilt value against the original.
func (s *FuzzFanCompileTestSuite) TestGeneratedFans(t *gotest.T, ctx *fanCompileCtx) {
	set := buildFuzzfanFixtureSet(t)
	gotest.NotEmpty(t, set.Targets, "expected a target for testdata/fuzzfan")

	// Rich exercises every literal-supported shape, so it must carry a
	// literal function.
	litFunc := "ƒ_fuzzlits_v1_Rich"
	gotest.Contains(t, set.Source, "func "+litFunc+"(")

	s.write(t, ctx, "go.mod", s.goMod(ctx, "fuzzfancheck"))
	fixtureTypes, err := os.ReadFile(filepath.Join("testdata", "fuzzfan", "types.go"))
	gotest.NoError(t, err)
	s.write(t, ctx, "types.go", fixtureTypes)
	check, err := os.ReadFile(filepath.Join("testdata", "fuzzfan", "check", "roundtrip_test.go"))
	gotest.NoError(t, err)
	s.write(t, ctx, "roundtrip_test.go", check)
	s.write(t, ctx, "fan_gen.go", []byte(
		"package fuzzfan\n\nimport (\n\t"+strings.Join(fanImports(set), "\n\t")+"\n)\n"+set.Source+fanExprsDecl(set)))

	t.It("prints a literal for every representative value and reconstructs each one", func(it *gotest.T) {
		s.write(it, ctx, "literal_print_test.go", []byte(literalPrintTestSrc(litFunc)))
		printOut := s.run(it, ctx, set, "test", "-run", "TestPrintFuzzLiterals", "-v", "./...")
		cases, err := parseLiteralCases(printOut)
		gotest.NoError(it, err, "output:\n%s", printOut)
		gotest.NotEmpty(it, cases, "output:\n%s", printOut)

		s.write(it, ctx, "literal_check_test.go", []byte(literalCheckTestSrc(cases)))
		s.run(it, ctx, set, "test", "./...")
	})
}

// TestCrossPackageLiteral proves a cross-package named basic renders as the
// qualified crossdep.ID, not the bare identifier that is out of scope where
// the literal is spliced, by building the emitted source against the
// crossdep package under the import path BuildFuzzTargets recorded.
func (s *FuzzFanCompileTestSuite) TestCrossPackageLiteral(t *gotest.T, ctx *fanCompileCtx) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), "TestFuzzCodec_CrossPackageBasic")
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	set, err := gotestgen.BuildFuzzTargets(pkg, spec.EffectiveTestSuites)
	gotest.NoError(t, err)

	t.It("qualifies the named basic", func(it *gotest.T) {
		gotest.Contains(it, set.Source, `"crossdep.ID("`)
		gotest.NotContains(it, set.Source, `"ID("`)
	})

	t.It("compiles against the declaring package", func(it *gotest.T) {
		s.write(it, ctx, "go.mod", s.goMod(ctx, "testpkg"))
		crossdepSrc, err := os.ReadFile(filepath.Join("testdata", "sources", "TestFuzzCodec_CrossDep", "test.go"))
		gotest.NoError(it, err)
		s.write(it, ctx, "TestFuzzCodec_CrossDep/test.go", crossdepSrc)

		imports := fanImports(set)
		for _, p := range set.PkgPaths {
			imports = append(imports, fmt.Sprintf("%q", p))
		}
		s.write(it, ctx, "fan_gen.go", []byte(
			"package check\n\nimport (\n\t"+strings.Join(imports, "\n\t")+"\n)\n"+set.Source+fanExprsDecl(set)))
		s.run(it, ctx, set, "build", "./...")
	})
}

// fanExprsDecl renders every target expression the wrapper hands to NewF as
// a package-level declaration, so the scratch module type-checks them
// against the generated functions the way the real generated file does.
func fanExprsDecl(set *gotestgen.FuzzTargetSet) string {
	var b strings.Builder
	b.WriteString("\nvar _ = []*gotest.FuzzTarget{\n")
	for _, f := range set.Targets {
		b.WriteString("\t")
		b.WriteString(f.Expr)
		b.WriteString(",\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// literalPrintTestSrc is phase 1's companion file: it calls litFunc over
// richCases() and prints each result tagged by index.
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

// parseLiteralCases extracts the index -> literal-text pairs phase 1 printed.
// SplitN caps at 3 fields, so a tab inside the literal (strconv.Quote escapes
// real ones) cannot corrupt the split.
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

// literalCheckTestSrc is phase 2's companion file: the raw literal text is
// spliced in verbatim as map-value initialisers, so it only compiles if that
// text is valid, self-contained Go.
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

// buildFuzzfanFixtureSet loads testdata/fuzzfan with Tests: true, so the
// suite in its _test.go file is visible, and emits its fans.
func buildFuzzfanFixtureSet(t *gotest.T) *gotestgen.FuzzTargetSet {
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, "./testdata/fuzzfan")
	gotest.NoError(t, err)
	for _, p := range pkgs {
		gotest.Empty(t, p.Errors, "package load errors for %s: %v", p.ID, p.Errors)
	}
	for _, p := range pkgs {
		if !strings.HasSuffix(p.ID, ".test]") || strings.HasSuffix(p.Name, "_test") {
			continue
		}
		c := gotestgen.NewCollector()
		result := c.CollectSuiteSpecs(p)
		gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
		spec, err := c.ApplyTestSuiteSpecs(result)
		gotest.NoError(t, err)
		set, err := gotestgen.BuildFuzzTargets(p, spec.EffectiveTestSuites)
		gotest.NoError(t, err)
		return set
	}
	gotest.Fail(t, "expected the ptest package variant for testdata/fuzzfan")
	return nil
}
