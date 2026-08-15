package gotestast_test

import (
	"go/token"
	"go/types"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
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
