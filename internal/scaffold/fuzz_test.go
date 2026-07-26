package scaffold //nolint:stdlib-test

import (
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInverseNameCandidates(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"Marshal", []string{"Unmarshal"}},
		{"Unmarshal", []string{"Marshal"}},
		{"Encode", []string{"Decode"}},
		{"Decode", []string{"Encode"}},
		{"Parse", []string{"Format", "String"}},
		{"Format", []string{"Parse"}},
		{"String", []string{"Parse"}},
		{"EncodeVarint", []string{"DecodeVarint"}},
		{"ParseJSON", []string{"FormatJSON"}},
		{"FormatJSON", []string{"ParseJSON"}},
		{"Render", nil}, // no table entry, no prefix match
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inverseNameCandidates(tc.name)
			if len(got) != len(tc.want) {
				t.Fatalf("inverseNameCandidates(%q) = %v, want %v", tc.name, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("inverseNameCandidates(%q)[%d] = %q, want %q", tc.name, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// newTestSignature builds a single-parameter *types.Signature: param -> result,
// optionally with a trailing error result.
func newTestSignature(param, result types.Type, withErr bool) *types.Signature {
	params := types.NewTuple(types.NewVar(0, nil, "in", param))
	var results *types.Tuple
	if withErr {
		results = types.NewTuple(
			types.NewVar(0, nil, "", result),
			types.NewVar(0, nil, "", errorType),
		)
	} else {
		results = types.NewTuple(types.NewVar(0, nil, "", result))
	}
	return types.NewSignature(nil, params, results, false)
}

func TestSignaturesInverse(t *testing.T) {
	strType := types.Typ[types.String]
	bytesType := types.NewSlice(types.Typ[types.Uint8])

	t.Run("Encode(string)([]byte,error) / Decode([]byte)(string,error) qualifies", func(t *testing.T) {
		f := newTestSignature(strType, bytesType, true)
		g := newTestSignature(bytesType, strType, true)
		fErr, gErr, ok := signaturesInverse(f, g)
		if !ok {
			t.Fatal("expected pair to qualify")
		}
		if !fErr || !gErr {
			t.Errorf("expected both sides to report returnsErr=true, got fErr=%v gErr=%v", fErr, gErr)
		}
	})

	t.Run("no-error variant qualifies", func(t *testing.T) {
		f := newTestSignature(strType, bytesType, false)
		g := newTestSignature(bytesType, strType, false)
		fErr, gErr, ok := signaturesInverse(f, g)
		if !ok {
			t.Fatal("expected pair to qualify")
		}
		if fErr || gErr {
			t.Errorf("expected both sides to report returnsErr=false, got fErr=%v gErr=%v", fErr, gErr)
		}
	})

	t.Run("asymmetric signatures don't qualify", func(t *testing.T) {
		// Both funcs take string and return ([]byte, error) — g never
		// consumes what f produced, so there's no round trip.
		f := newTestSignature(strType, bytesType, true)
		g := newTestSignature(strType, bytesType, true)
		if _, _, ok := signaturesInverse(f, g); ok {
			t.Fatal("expected asymmetric signatures to not qualify")
		}
	})

	t.Run("bare-error-only result doesn't qualify (no B to round-trip)", func(t *testing.T) {
		f := newTestSignature(strType, errorType, false) // (string) error
		g := newTestSignature(bytesType, strType, true)
		if _, _, ok := signaturesInverse(f, g); ok {
			t.Fatal("expected bare-error result to not qualify")
		}
	})

	t.Run("named types compared with types.Identical", func(t *testing.T) {
		pkg := types.NewPackage("example.com/x", "x")
		idA := types.NewNamed(types.NewTypeName(0, pkg, "IDA", nil), strType, nil)
		idB := types.NewNamed(types.NewTypeName(0, pkg, "IDB", nil), strType, nil)

		// f: string -> IDA ; g: IDA -> string  => qualifies (identical named type)
		f := newTestSignature(strType, idA, false)
		g := newTestSignature(idA, strType, false)
		if _, _, ok := signaturesInverse(f, g); !ok {
			t.Fatal("expected pair to qualify for identical named type")
		}

		// g2: IDB -> string — same underlying as IDA but a distinct named
		// type, must NOT qualify.
		g2 := newTestSignature(idB, strType, false)
		if _, _, ok := signaturesInverse(f, g2); ok {
			t.Fatal("expected pair to NOT qualify for differing named types despite identical underlying type")
		}
	})
}

func TestNativeFuzzable(t *testing.T) {
	pkg := types.NewPackage("example.com/x", "x")
	named := types.NewNamed(types.NewTypeName(0, pkg, "UserID", nil), types.Typ[types.String], nil)
	structType := types.NewStruct(nil, nil)

	tests := []struct {
		name     string
		typ      types.Type
		wantZero string
		wantOK   bool
	}{
		{"string", types.Typ[types.String], `""`, true},
		{"bool", types.Typ[types.Bool], "false", true},
		{"int", types.Typ[types.Int], "0", true},
		{"int64", types.Typ[types.Int64], "0", true},
		{"uint8/byte", types.Typ[types.Uint8], "0", true},
		{"int32/rune", types.Typ[types.Int32], "0", true},
		{"float64", types.Typ[types.Float64], "0", true},
		{"[]byte", types.NewSlice(types.Typ[types.Uint8]), `[]byte("")`, true},
		{"named string type", named, "", false},
		{"struct", structType, "", false},
		{"[]int (not []byte)", types.NewSlice(types.Typ[types.Int]), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			zero, ok := nativeFuzzable(tc.typ)
			if ok != tc.wantOK {
				t.Fatalf("nativeFuzzable(%s): ok = %v, want %v", tc.name, ok, tc.wantOK)
			}
			if ok && zero != tc.wantZero {
				t.Errorf("nativeFuzzable(%s): zero = %q, want %q", tc.name, zero, tc.wantZero)
			}
		})
	}
}

func TestIntrospectFuzzTarget(t *testing.T) {
	t.Run("Encode has a compatible Decode inverse", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "Encode")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		if !target.Fuzzable {
			t.Fatal("expected string param to be fuzzable")
		}
		if target.Pair == nil {
			t.Fatal("expected Decode to be found as an inverse pair")
		}
		if target.Pair.Name != "Decode" {
			t.Errorf("Pair.Name = %q, want %q", target.Pair.Name, "Decode")
		}
		if !target.Pair.FuncReturnsErr || !target.Pair.InverseReturnsErr {
			t.Error("expected both Encode and Decode to be recorded as error-returning")
		}
	})

	t.Run("Render has no matching inverse", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "Render")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		if !target.Fuzzable {
			t.Fatal("expected int param to be fuzzable")
		}
		if target.Pair != nil {
			t.Fatalf("expected no inverse pair, got %+v", target.Pair)
		}
	})

	t.Run("ApplyConfig's struct param is not fuzzable", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "ApplyConfig")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		if target.Fuzzable {
			t.Fatal("expected struct param to not be fuzzable")
		}
	})

	t.Run("unknown function errors", func(t *testing.T) {
		if _, err := IntrospectFuzzTarget("./testdata/sampletype", "DoesNotExist"); err == nil {
			t.Fatal("expected error for unknown function")
		}
	})

	t.Run("method-shaped lookup errors", func(t *testing.T) {
		// UserService is a type, not a function.
		if _, err := IntrospectFuzzTarget("./testdata/sampletype", "UserService"); err == nil {
			t.Fatal("expected error for non-function target")
		}
	})
}

func TestGenerateFuzzScaffold(t *testing.T) {
	t.Run("round-trip pair", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "Encode")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		out, status, err := GenerateFuzzScaffold(target)
		if err != nil {
			t.Fatalf("GenerateFuzzScaffold failed: %v", err)
		}
		if status != "" {
			t.Errorf("expected empty status for a found pair, got %q", status)
		}
		src := string(out)
		if !strings.Contains(src, "gotest.Fuzz(") {
			t.Error("missing gotest.Fuzz( call")
		}
		if !strings.Contains(src, "Encode") || !strings.Contains(src, "Decode") {
			t.Error("missing reference to both Encode and Decode")
		}
		if !strings.Contains(src, "gotest.Equal(t, in, decoded) // round-trip property") {
			t.Error("missing round-trip assertion")
		}
		if !strings.Contains(src, "func (s *EncodeTestSuite) FuzzEncode(f *gotest.F)") {
			t.Error("missing FuzzEncode method")
		}
	})

	t.Run("no pair found falls back to crash-safety skeleton", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "Render")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		out, status, err := GenerateFuzzScaffold(target)
		if err != nil {
			t.Fatalf("GenerateFuzzScaffold failed: %v", err)
		}
		if status != "no inverse pair found for Render — generated crash-safety skeleton" {
			t.Errorf("unexpected status: %q", status)
		}
		src := string(out)
		if !strings.Contains(src, "gotest.Fuzz(") {
			t.Error("missing gotest.Fuzz( call")
		}
		if !strings.Contains(src, "Render(in)") {
			t.Error("missing call to Render")
		}
		if strings.Contains(src, "gotest.Equal") {
			t.Error("crash-safety skeleton should not assert a round-trip property")
		}
	})

	t.Run("non-fuzzable param falls back to a TODO stub", func(t *testing.T) {
		target, err := IntrospectFuzzTarget("./testdata/sampletype", "ApplyConfig")
		if err != nil {
			t.Fatalf("IntrospectFuzzTarget failed: %v", err)
		}
		out, status, err := GenerateFuzzScaffold(target)
		if err != nil {
			t.Fatalf("GenerateFuzzScaffold failed: %v", err)
		}
		wantStatus := "sampletype.Config is not natively fuzzable for ApplyConfig — generated TODO stub (struct fuzzing is not yet supported)"
		if status != wantStatus {
			t.Errorf("status = %q, want %q", status, wantStatus)
		}
		src := string(out)
		if strings.Contains(src, "gotest.Fuzz(") {
			t.Error("non-fuzzable stub should not call gotest.Fuzz")
		}
		if !strings.Contains(src, "not natively fuzzable") {
			t.Error("missing not-fuzzable comment")
		}
	})
}

// fuzzCheckModOnce/fuzzCheckGoMod/fuzzCheckGoSum cache a single tidied
// scratch module (go.mod + go.sum) that replaces github.com/mvrahden/go-test
// with this worktree, shared across TestGenerateFuzzScaffold_Compiles'
// subtests so "go mod tidy" — the slow part — runs exactly once. Mirrors
// the module+replace pattern internal/gotestgen/export_test.go already
// uses to load real Go packages against this module in tests.
var (
	fuzzCheckModOnce sync.Once
	fuzzCheckGoMod   []byte
	fuzzCheckGoSum   []byte
	fuzzCheckErr     error
)

func fuzzCheckModule(t *testing.T) (goMod, goSum []byte) {
	t.Helper()
	fuzzCheckModOnce.Do(func() {
		repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			fuzzCheckErr = err
			return
		}
		scratch, err := os.MkdirTemp("", "gotest-fuzzcheck-mod-*")
		if err != nil {
			fuzzCheckErr = err
			return
		}
		defer os.RemoveAll(scratch)

		modSrc := "module fuzzcheck\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + repoRoot + "\n"
		if err := os.WriteFile(filepath.Join(scratch, "go.mod"), []byte(modSrc), 0600); err != nil {
			fuzzCheckErr = err
			return
		}
		stub := "package fuzzcheck\n\nimport _ \"github.com/mvrahden/go-test/pkg/gotest\"\n"
		if err := os.WriteFile(filepath.Join(scratch, "stub.go"), []byte(stub), 0600); err != nil {
			fuzzCheckErr = err
			return
		}

		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = scratch
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			fuzzCheckErr = fmt.Errorf("go mod tidy: %w\n%s", err, out)
			return
		}

		fuzzCheckGoMod, fuzzCheckErr = os.ReadFile(filepath.Join(scratch, "go.mod"))
		if fuzzCheckErr != nil {
			return
		}
		// go.sum may not exist at all when the module has no third-party
		// dependencies beyond the replaced local one (pkg/gotest doesn't).
		fuzzCheckGoSum, _ = os.ReadFile(filepath.Join(scratch, "go.sum"))
	})
	if fuzzCheckErr != nil {
		t.Fatalf("preparing fuzz-check scratch module: %v", fuzzCheckErr)
	}
	return fuzzCheckGoMod, fuzzCheckGoSum
}

// vetGeneratedFuzzFile writes src into an isolated module (built from the
// shared, already-tidied go.mod/go.sum) alongside the sampletype fixture's
// source, then runs "go vet ./..." — this is a real, full type-check of the
// generated skeleton against the actual gotest.F/gotest.T/gotest.Equal/
// gotest.NoError API in this worktree, not a substring guess. It fails
// loudly (with go vet's output) if the skeleton doesn't compile.
func vetGeneratedFuzzFile(t *testing.T, filename string, src []byte) {
	t.Helper()
	goMod, goSum := fuzzCheckModule(t)

	codecSrc, err := os.ReadFile("testdata/sampletype/codec.go")
	if err != nil {
		t.Fatalf("reading fixture source: %v", err)
	}

	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	write("go.mod", goMod)
	if len(goSum) > 0 {
		write("go.sum", goSum)
	}
	write("codec.go", codecSrc)
	write(filename, src)

	vet := exec.Command("go", "vet", "./...")
	vet.Dir = dir
	vet.Env = append(os.Environ(), "GOWORK=off")
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("generated fuzz skeleton does not compile (go vet):\n%s\n--- generated source ---\n%s", out, src)
	}
}

// TestGenerateFuzzScaffold_Compiles type-checks the actual generated output
// for all three skeleton shapes (round-trip pair, no-pair crash-safety,
// not-fuzzable stub) against the real gotest package in this worktree —
// substring assertions alone can't catch a bug like referencing a
// gotest.TestSuite type that doesn't exist.
func TestGenerateFuzzScaffold_Compiles(t *testing.T) {
	for _, funcName := range []string{"Encode", "Render", "ApplyConfig"} {
		t.Run(funcName, func(t *testing.T) {
			target, err := IntrospectFuzzTarget("./testdata/sampletype", funcName)
			if err != nil {
				t.Fatalf("IntrospectFuzzTarget(%s): %v", funcName, err)
			}
			out, _, err := GenerateFuzzScaffold(target)
			if err != nil {
				t.Fatalf("GenerateFuzzScaffold(%s): %v", funcName, err)
			}
			vetGeneratedFuzzFile(t, strings.ToLower(funcName)+"_generated.go", out)
		})
	}
}
