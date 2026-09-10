package gotestast_test

import (
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// FuzzArgsTestSuite pins the type predicates every fan-out consumer keys
// off: which types gotest hands to the engine untouched, and which types'
// corpus entries depend on a field layout.
type FuzzArgsTestSuite struct{}

func named(name string, under types.Type) *types.Named {
	return types.NewNamed(types.NewTypeName(token.NoPos, nil, name, nil), under, nil)
}

func (s *FuzzArgsTestSuite) TestPassthroughFuzzType(t *gotest.T) {
	str := types.Typ[types.String]
	boolean := types.Typ[types.Bool]
	byteSlice := types.NewSlice(types.Typ[types.Uint8])

	t.It("accepts exactly the unnamed string, bool, and []byte", func(it *gotest.T) {
		gotest.True(it, gotestast.PassthroughFuzzType(str))
		gotest.True(it, gotestast.PassthroughFuzzType(boolean))
		gotest.True(it, gotestast.PassthroughFuzzType(byteSlice))
	})

	t.It("rejects every number — numbers ride as fixed-width []byte leaves", func(it *gotest.T) {
		for _, k := range []types.BasicKind{types.Int, types.Int8, types.Uint64, types.Float32, types.Float64} {
			gotest.False(it, gotestast.PassthroughFuzzType(types.Typ[k]), "%s must fan", types.Typ[k])
		}
	})

	t.It("rejects named types over pass-through kinds — they need the conversion", func(it *gotest.T) {
		gotest.False(it, gotestast.PassthroughFuzzType(named("Topic", str)))
		gotest.False(it, gotestast.PassthroughFuzzType(named("Blob", byteSlice)))
		gotest.False(it, gotestast.PassthroughFuzzType(named("Flag", boolean)))
	})

	t.It("sees through an alias", func(it *gotest.T) {
		alias := types.NewAlias(types.NewTypeName(token.NoPos, nil, "S", nil), str)
		gotest.True(it, gotestast.PassthroughFuzzType(alias))
	})

	t.It("rejects composite shapes", func(it *gotest.T) {
		gotest.False(it, gotestast.PassthroughFuzzType(types.NewStruct(nil, nil)))
		gotest.False(it, gotestast.PassthroughFuzzType(types.NewSlice(str)))
		gotest.False(it, gotestast.PassthroughFuzzType(types.NewPointer(str)))
	})
}

func (s *FuzzArgsTestSuite) TestFuzzCorpusShapeBound(t *gotest.T) {
	str := types.Typ[types.String]
	field := types.NewField(token.NoPos, nil, "Name", str, false)
	structT := named("Req", types.NewStruct([]*types.Var{field}, nil))

	t.It("is true for structs, pointers, arrays, and non-byte slices", func(it *gotest.T) {
		gotest.True(it, gotestast.FuzzCorpusShapeBound(structT))
		gotest.True(it, gotestast.FuzzCorpusShapeBound(types.NewPointer(str)))
		gotest.True(it, gotestast.FuzzCorpusShapeBound(types.NewArray(str, 3)))
		gotest.True(it, gotestast.FuzzCorpusShapeBound(types.NewSlice(str)))
	})

	t.It("is false for every scalar kind and for byte slices, named or not", func(it *gotest.T) {
		gotest.False(it, gotestast.FuzzCorpusShapeBound(str))
		gotest.False(it, gotestast.FuzzCorpusShapeBound(types.Typ[types.Int]))
		gotest.False(it, gotestast.FuzzCorpusShapeBound(named("Age", types.Typ[types.Int])))
		gotest.False(it, gotestast.FuzzCorpusShapeBound(types.NewSlice(types.Typ[types.Uint8])))
		gotest.False(it, gotestast.FuzzCorpusShapeBound(named("Blob", types.NewSlice(types.Typ[types.Uint8]))))
	})
}

// loadTestdataPkg loads one testdata package's test-binary variant, the
// shape gotestgen.LoadPackages hands the collector.
func loadTestdataPkg(t *testing.T, pattern string) *packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedModule | packages.NeedSyntax | packages.NeedName |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: true,
		Dir:   ".",
	}
	pkgs, err := packages.Load(cfg, pattern)
	gotest.NoError(t, err)
	for _, p := range pkgs {
		gotest.Empty(t, p.Errors, "package load errors for %s: %v", p.ID, p.Errors)
		if strings.HasSuffix(p.ID, ".test]") && !strings.HasSuffix(p.Name, "_test") {
			return p
		}
	}
	t.Fatalf("no test-binary package variant for %s", pattern)
	return nil
}

func (s *FuzzArgsTestSuite) TestCollectFuzzCalls(t *gotest.T) {
	pkg := loadTestdataPkg(t.T(), "./testdata/fuzzcalls")
	all := collectHarvestSuites(t.T(), pkg)
	only := func(name string) gotestast.TestSuiteSpecSet {
		for _, ts := range all {
			if ts.Identifier() == name {
				return gotestast.TestSuiteSpecSet{ts}
			}
		}
		gotest.True(t, false, "suite %s not in fixture", name)
		return nil
	}
	typeNames := func(c gotestast.FuzzCall) []string {
		var out []string
		for _, ty := range c.Types {
			out = append(out, types.TypeString(ty, func(p *types.Package) string { return "" }))
		}
		return out
	}

	t.It("reads the callback's parameters after *gotest.T, in engine order", func(it *gotest.T) {
		calls, err := gotestast.CollectFuzzCalls(pkg, only("OkTestSuite"))
		gotest.NoError(it, err)
		gotest.Len(it, calls, 1)
		gotest.Equal(it, "FuzzOkTestSuite_FuzzPair", calls[0].FuncName)
		gotest.Equal(it, []string{"Header", "string"}, typeNames(calls[0]))
	})

	t.It("accepts a method value as the callback", func(it *gotest.T) {
		calls, err := gotestast.CollectFuzzCalls(pkg, only("NamedTestSuite"))
		gotest.NoError(it, err)
		gotest.Len(it, calls, 1)
		gotest.Equal(it, []string{"int"}, typeNames(calls[0]))
	})

	t.When("a fuzz method is malformed", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Desc  string
			suite string
			want  string
		}{
			{Desc: "two f.Fuzz calls", suite: "TwiceTestSuite", want: "FuzzTwice calls f.Fuzz more than once"},
			{Desc: "no f.Fuzz call", suite: "NoneTestSuite", want: "FuzzNone never calls f.Fuzz"},
			{Desc: "a non-function callback", suite: "NotFuncTestSuite", want: "FuzzNotFunc: f.Fuzz callback must be a func(*gotest.T, ...), got string"},
			{Desc: "a callback without *gotest.T", suite: "NoTTestSuite", want: "FuzzNoT: f.Fuzz callback's first parameter must be *gotest.T"},
		}) {
			sub.It("names the method and the reason for "+tc.Desc, func(it *gotest.T) {
				_, err := gotestast.CollectFuzzCalls(pkg, only(tc.suite))
				gotest.ErrorContains(it, err, tc.want)
			})
		}
	})
}
