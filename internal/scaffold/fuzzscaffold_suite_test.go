package scaffold_test

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/scaffold"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzScaffoldTestSuite covers `scaffold --fuzz`: inverse-pair detection,
// fuzzable parameter types, target introspection and the generated skeleton,
// which is type-checked against the real gotest package in this worktree.
//
//nolint:lifecycle-pair // BeforeAll only caches bytes; its scratch module lives under t.TempDir()
type FuzzScaffoldTestSuite struct {
	goMod []byte
	goSum []byte
}

func (s *FuzzScaffoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type fuzzScaffoldCtx struct{}

// BeforeAll tidies one scratch module that replaces github.com/mvrahden/go-test
// with this worktree, so the slow `go mod tidy` runs once per suite.
func (s *FuzzScaffoldTestSuite) BeforeAll(t *gotest.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	scratch := t.TempDir()

	modSrc := "module fuzzcheck\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + repoRoot + "\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(scratch, "go.mod"), []byte(modSrc), 0600))
	stub := "package fuzzcheck\n\nimport _ \"github.com/mvrahden/go-test/pkg/gotest\"\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(scratch, "stub.go"), []byte(stub), 0600))

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go mod tidy: %s", out)

	s.goMod, err = os.ReadFile(filepath.Join(scratch, "go.mod"))
	gotest.NoError(t, err)
	// go.sum is absent when nothing beyond the replaced module is required.
	s.goSum, _ = os.ReadFile(filepath.Join(scratch, "go.sum"))
}

func (s *FuzzScaffoldTestSuite) BeforeEach(t *gotest.T) *fuzzScaffoldCtx { return &fuzzScaffoldCtx{} }

// newSignature builds a single-parameter signature param -> result, with a
// trailing error result when withErr is set.
func newSignature(param, result types.Type, withErr bool) *types.Signature {
	params := types.NewTuple(types.NewVar(0, nil, "in", param))
	var results *types.Tuple
	if withErr {
		results = types.NewTuple(
			types.NewVar(0, nil, "", result),
			types.NewVar(0, nil, "", scaffold.ExportErrorType),
		)
	} else {
		results = types.NewTuple(types.NewVar(0, nil, "", result))
	}
	return types.NewSignature(nil, params, results, false) //nolint:staticcheck // SA1019: type parameters irrelevant for this fixture signature
}

func (s *FuzzScaffoldTestSuite) TestInverseNameCandidates(t *gotest.T, _ *fuzzScaffoldCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		want []string
	}{
		{Desc: "Marshal", want: []string{"Unmarshal"}},
		{Desc: "Unmarshal", want: []string{"Marshal"}},
		{Desc: "Encode", want: []string{"Decode"}},
		{Desc: "Decode", want: []string{"Encode"}},
		{Desc: "Parse", want: []string{"Format", "String"}},
		{Desc: "Format", want: []string{"Parse"}},
		{Desc: "String", want: []string{"Parse"}},
		{Desc: "EncodeVarint", want: []string{"DecodeVarint"}},
		{Desc: "ParseJSON", want: []string{"FormatJSON"}},
		{Desc: "FormatJSON", want: []string{"ParseJSON"}},
		{Desc: "Render", want: nil}, // no table entry, no prefix match
	}) {
		gotest.Equal(sub, tc.want, scaffold.ExportInverseNameCandidates(tc.Desc))
	}
}

func (s *FuzzScaffoldTestSuite) TestSignaturesInverse(t *gotest.T, _ *fuzzScaffoldCtx) {
	strType := types.Typ[types.String]
	bytesType := types.NewSlice(types.Typ[types.Uint8])

	t.It("qualifies Encode(string)([]byte,error) with Decode([]byte)(string,error)", func(it *gotest.T) {
		f := newSignature(strType, bytesType, true)
		g := newSignature(bytesType, strType, true)
		fErr, gErr, ok := scaffold.ExportSignaturesInverse(f, g)
		gotest.True(it, ok)
		gotest.True(it, fErr)
		gotest.True(it, gErr)
	})

	t.It("qualifies the no-error variant", func(it *gotest.T) {
		f := newSignature(strType, bytesType, false)
		g := newSignature(bytesType, strType, false)
		fErr, gErr, ok := scaffold.ExportSignaturesInverse(f, g)
		gotest.True(it, ok)
		gotest.False(it, fErr)
		gotest.False(it, gErr)
	})

	t.It("rejects asymmetric signatures", func(it *gotest.T) {
		// g never consumes what f produced, so there is no round trip.
		f := newSignature(strType, bytesType, true)
		g := newSignature(strType, bytesType, true)
		_, _, ok := scaffold.ExportSignaturesInverse(f, g)
		gotest.False(it, ok)
	})

	t.It("rejects a bare error result, which leaves nothing to round-trip", func(it *gotest.T) {
		f := newSignature(strType, scaffold.ExportErrorType, false)
		g := newSignature(bytesType, strType, true)
		_, _, ok := scaffold.ExportSignaturesInverse(f, g)
		gotest.False(it, ok)
	})

	t.It("compares named types by identity, not by underlying type", func(it *gotest.T) {
		pkg := types.NewPackage("example.com/x", "x")
		idA := types.NewNamed(types.NewTypeName(0, pkg, "IDA", nil), strType, nil)
		idB := types.NewNamed(types.NewTypeName(0, pkg, "IDB", nil), strType, nil)

		f := newSignature(strType, idA, false)
		g := newSignature(idA, strType, false)
		_, _, ok := scaffold.ExportSignaturesInverse(f, g)
		gotest.True(it, ok)

		g2 := newSignature(idB, strType, false)
		_, _, ok = scaffold.ExportSignaturesInverse(f, g2)
		gotest.False(it, ok)
	})
}

func (s *FuzzScaffoldTestSuite) TestNativeFuzzable(t *gotest.T, _ *fuzzScaffoldCtx) {
	pkg := types.NewPackage("example.com/x", "x")
	named := types.NewNamed(types.NewTypeName(0, pkg, "UserID", nil), types.Typ[types.String], nil)

	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		typ      types.Type
		wantZero string
		wantOK   bool
	}{
		{Desc: "string", typ: types.Typ[types.String], wantZero: `""`, wantOK: true},
		{Desc: "bool", typ: types.Typ[types.Bool], wantZero: "false", wantOK: true},
		{Desc: "int", typ: types.Typ[types.Int], wantZero: "0", wantOK: true},
		{Desc: "int64", typ: types.Typ[types.Int64], wantZero: "0", wantOK: true},
		{Desc: "uint8 (byte)", typ: types.Typ[types.Uint8], wantZero: "0", wantOK: true},
		{Desc: "int32 (rune)", typ: types.Typ[types.Int32], wantZero: "0", wantOK: true},
		{Desc: "float64", typ: types.Typ[types.Float64], wantZero: "0", wantOK: true},
		{Desc: "[]byte", typ: types.NewSlice(types.Typ[types.Uint8]), wantZero: `[]byte("")`, wantOK: true},
		{Desc: "named string type", typ: named},
		{Desc: "struct", typ: types.NewStruct(nil, nil)},
		{Desc: "[]int, not []byte", typ: types.NewSlice(types.Typ[types.Int])},
	}) {
		zero, ok := scaffold.ExportNativeFuzzable(tc.typ)
		gotest.Equal(sub, tc.wantOK, ok)
		gotest.Equal(sub, tc.wantZero, zero)
	}
}

func (s *FuzzScaffoldTestSuite) TestIntrospectFuzzTarget(t *gotest.T, _ *fuzzScaffoldCtx) {
	t.It("pairs Encode with its Decode inverse", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "Encode")
		gotest.NoError(it, err)
		gotest.True(it, target.Fuzzable)
		gotest.NotZero(it, target.Pair)
		gotest.Equal(it, "Decode", target.Pair.Name)
		gotest.True(it, target.Pair.FuncReturnsErr)
		gotest.True(it, target.Pair.InverseReturnsErr)
	})

	t.It("finds no inverse for Render", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "Render")
		gotest.NoError(it, err)
		gotest.True(it, target.Fuzzable)
		gotest.Zero(it, target.Pair)
	})

	t.It("treats ApplyConfig's struct parameter as fuzzable with a package-relative type", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "ApplyConfig")
		gotest.NoError(it, err)
		gotest.True(it, target.Fuzzable, "reject: %s", target.RejectReason)
		gotest.Equal(it, "Config", target.ParamTypeStr)
		gotest.Equal(it, "Config{}", target.ZeroLiteral)
	})

	t.It("rejects ApplyOptions' map parameter with the emitter's reason", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "ApplyOptions")
		gotest.NoError(it, err)
		gotest.False(it, target.Fuzzable)
		gotest.Contains(it, target.RejectReason, "maps have no canonical encoding")
	})

	t.It("errors on an unknown function", func(it *gotest.T) {
		_, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "DoesNotExist")
		gotest.Error(it, err)
	})

	t.It("errors on a type name", func(it *gotest.T) {
		_, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "UserService")
		gotest.Error(it, err)
	})
}

func (s *FuzzScaffoldTestSuite) TestGenerateFuzzScaffold(t *gotest.T, _ *fuzzScaffoldCtx) {
	t.It("asserts the round trip for a pair", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "Encode")
		gotest.NoError(it, err)
		out, status, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(it, err)
		gotest.Empty(it, status)
		src := string(out)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "Encode")
		gotest.Contains(it, src, "Decode")
		gotest.Contains(it, src, "gotest.Equal(t, in, decoded) // round-trip property")
		gotest.Contains(it, src, "func (s *EncodeTestSuite) FuzzEncode(f *gotest.F)")
	})

	t.It("falls back to a crash-safety skeleton without a pair", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "Render")
		gotest.NoError(it, err)
		out, status, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(it, err)
		gotest.Equal(it, "no inverse pair found for Render — generated crash-safety skeleton", status)
		src := string(out)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "Render(in)")
		gotest.NotContains(it, src, "gotest.Equal")
	})

	t.It("gives a fuzzable struct parameter a real skeleton with a typed zero seed", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "ApplyConfig")
		gotest.NoError(it, err)
		out, status, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(it, err)
		gotest.Equal(it, "no inverse pair found for ApplyConfig — generated crash-safety skeleton", status)
		src := string(out)
		gotest.Contains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "f.Add(Config{})")
	})

	t.It("stubs a rejected parameter with the emitter's reason", func(it *gotest.T) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", "ApplyOptions")
		gotest.NoError(it, err)
		out, status, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(it, err)
		gotest.True(it, strings.HasPrefix(status, "cannot fuzz map[string]string for ApplyOptions — generated TODO stub: "), "status: %q", status)
		gotest.Contains(it, status, "maps have no canonical encoding")
		src := string(out)
		gotest.NotContains(it, src, "f.Fuzz(")
		gotest.Contains(it, src, "maps have no canonical encoding")
	})
}

// vetGenerated writes src beside the sampletype fixture into a module built
// from the tidied go.mod/go.sum and runs `go vet ./...`: a full type-check of
// the skeleton against the real gotest API, not a substring guess.
func (s *FuzzScaffoldTestSuite) vetGenerated(t *gotest.T, dir, filename string, src []byte) {
	codecSrc, err := os.ReadFile("testdata/sampletype/codec.go")
	gotest.NoError(t, err)

	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), s.goMod, 0600))
	if len(s.goSum) > 0 {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), s.goSum, 0600))
	}
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "codec.go"), codecSrc, 0600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, filename), src, 0600))

	vet := exec.Command("go", "vet", "./...")
	vet.Dir = dir
	vet.Env = append(os.Environ(), "GOWORK=off")
	out, err := vet.CombinedOutput()
	gotest.NoError(t, err, "generated fuzz skeleton does not compile (go vet):\n%s\n--- generated source ---\n%s", out, src)
}

// TestGeneratedScaffoldCompiles type-checks every skeleton shape (pair,
// crash-safety, struct, rejected stub); substring checks cannot catch a
// reference to a type that does not exist.
func (s *FuzzScaffoldTestSuite) TestGeneratedScaffoldCompiles(t *gotest.T, _ *fuzzScaffoldCtx) {
	for sub, funcName := range gotest.Each(t, []string{"Encode", "Render", "ApplyConfig", "ApplyOptions"}) {
		target, err := scaffold.IntrospectFuzzTarget("./testdata/sampletype", funcName)
		gotest.NoError(sub, err)
		out, _, err := scaffold.GenerateFuzzScaffold(target)
		gotest.NoError(sub, err)
		s.vetGenerated(sub, sub.TempDir(), fmt.Sprintf("%s_generated.go", strings.ToLower(funcName)), out)
	}
}
