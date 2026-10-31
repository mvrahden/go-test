package typeclass_test

import (
	"go/token"
	"go/types"

	"github.com/mvrahden/go-test/internal/typeclass"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TypeClassTestSuite pins the one classification every consumer keys off —
// discovery, the generator and the lint rules — over the type shapes that
// have each fooled a copy of it: names, aliases, type parameters,
// unsafe.Pointer, interfaces and error implementers.
type TypeClassTestSuite struct{}

func (s *TypeClassTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var (
	errorType  = types.Universe.Lookup("error").Type()
	stringType = types.Typ[types.String]
	intType    = types.Typ[types.Int]
)

func name(n string) *types.TypeName { return types.NewTypeName(token.NoPos, nil, n, nil) }

func named(n string, under types.Type) *types.Named { return types.NewNamed(name(n), under, nil) }

func alias(n string, of types.Type) types.Type { return types.NewAlias(name(n), of) }

// constrained builds a type parameter over the constraint interface.
func constrained(n string, constraint types.Type) *types.TypeParam {
	return types.NewTypeParam(name(n), constraint)
}

// union is the constraint interface holding one union of the given terms.
func union(terms ...types.Type) *types.Interface {
	ts := make([]*types.Term, len(terms))
	for i, t := range terms {
		ts[i] = types.NewTerm(true, t)
	}
	return types.NewInterfaceType(nil, []types.Type{types.NewUnion(ts)}).Complete()
}

// errorString is a named struct whose pointer implements error.
func errorString() *types.Named {
	n := named("errorString", types.NewStruct(nil, nil))
	recv := types.NewVar(token.NoPos, nil, "e", types.NewPointer(n))
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType))
	n.AddMethod(types.NewFunc(token.NoPos, nil, "Error", types.NewSignatureType(recv, nil, nil, nil, results, false)))
	return n
}

// errorLike is an interface with exactly error's method set, under a name.
func errorLike() types.Type {
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType))
	m := types.NewFunc(token.NoPos, nil, "Error", types.NewSignatureType(nil, nil, nil, nil, results, false))
	return named("errorLike", types.NewInterfaceType([]*types.Func{m}, nil).Complete())
}

// codedError is an interface with error's method and one more.
func codedError() types.Type {
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", stringType))
	errM := types.NewFunc(token.NoPos, nil, "Error", types.NewSignatureType(nil, nil, nil, nil, results, false))
	codeM := types.NewFunc(token.NoPos, nil, "Code", types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "", intType)), false))
	return named("codedError", types.NewInterfaceType([]*types.Func{errM, codeM}, nil).Complete())
}

type classRow struct {
	Desc       string
	typ        types.Type
	nilable    bool
	comparable bool
	core       types.Type // nil: no core type
}

func (s *TypeClassTestSuite) TestNilableAndComparable(t *gotest.T) {
	sliceOfInt := types.NewSlice(intType)
	ptrToInt := types.NewPointer(intType)
	anyParam := constrained("A", types.Universe.Lookup("any").Type())
	comparableParam := constrained("C", types.Universe.Lookup("comparable").Type())
	sliceParam := constrained("S", union(sliceOfInt))
	numberParam := constrained("N", union(intType, types.Typ[types.Float64]))
	t.When("a type is classified", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []classRow{
			{Desc: "string is comparable, not nilable", typ: stringType, comparable: true, core: stringType},
			{Desc: "a slice is nilable, not comparable", typ: sliceOfInt, nilable: true, core: sliceOfInt},
			{Desc: "a map is nilable, not comparable", typ: types.NewMap(stringType, intType), nilable: true, core: types.NewMap(stringType, intType)},
			{Desc: "a func is nilable, not comparable", typ: types.NewSignatureType(nil, nil, nil, nil, nil, false), nilable: true, core: types.NewSignatureType(nil, nil, nil, nil, nil, false)},
			{Desc: "a channel is nilable and comparable", typ: types.NewChan(types.SendRecv, intType), nilable: true, comparable: true, core: types.NewChan(types.SendRecv, intType)},
			{Desc: "a pointer is nilable and comparable", typ: ptrToInt, nilable: true, comparable: true, core: ptrToInt},
			{Desc: "an interface is nilable and comparable", typ: types.Universe.Lookup("any").Type(), nilable: true, comparable: true, core: types.Universe.Lookup("any").Type().Underlying()},
			{Desc: "unsafe.Pointer is nilable and comparable", typ: types.Typ[types.UnsafePointer], nilable: true, comparable: true, core: types.Typ[types.UnsafePointer]},
			{Desc: "a named slice classifies as its underlying slice", typ: named("IDs", sliceOfInt), nilable: true, core: sliceOfInt},
			{Desc: "an alias of a pointer classifies as the pointer", typ: alias("P", ptrToInt), nilable: true, comparable: true, core: ptrToInt},
			{Desc: "an alias of an alias still resolves", typ: alias("Q", alias("P", sliceOfInt)), nilable: true, core: sliceOfInt},
			{Desc: "a struct of comparables is comparable, not nilable", typ: types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "n", intType, false)}, nil), comparable: true, core: types.NewStruct([]*types.Var{types.NewField(token.NoPos, nil, "n", intType, false)}, nil)},
			{Desc: "an array of slices is neither", typ: types.NewArray(sliceOfInt, 2), core: types.NewArray(sliceOfInt, 2)},
			{Desc: "a type parameter over ~[]int takes the slice's class", typ: sliceParam, nilable: true, core: sliceOfInt},
			{Desc: "a type parameter over any has no core type and no class", typ: anyParam},
			{Desc: "a type parameter over comparable is comparable, not nilable", typ: comparableParam, comparable: true},
			{Desc: "a type parameter over int | float64 has no core type", typ: numberParam, comparable: true},
		}) {
			gotest.Equal(it, row.nilable, typeclass.Nilable(row.typ), "Nilable(%s)", row.typ)
			gotest.Equal(it, row.comparable, typeclass.Comparable(row.typ), "Comparable(%s)", row.typ)
			core := typeclass.CoreType(row.typ)
			if row.core == nil {
				gotest.Zero(it, core, "CoreType(%s)", row.typ)
			} else {
				gotest.True(it, core != nil && types.Identical(row.core, core), "CoreType(%s) = %v, want %v", row.typ, core, row.core)
			}
		}
	})
}

func (s *TypeClassTestSuite) TestErrors(t *gotest.T) {
	es := errorString()
	t.When("a type is held against error", func(w *gotest.T) {
		for it, row := range gotest.Each(w, []struct {
			Desc        string
			typ         types.Type
			isInterface bool
			implements  bool
		}{
			{Desc: "error itself is the interface and implements it", typ: errorType, isInterface: true, implements: true},
			{Desc: "an alias of error is the interface", typ: alias("E", errorType), isInterface: true, implements: true},
			{Desc: "a named interface with error's method set is the interface", typ: errorLike(), isInterface: true, implements: true},
			{Desc: "an interface with more methods implements error but is not it", typ: codedError(), implements: true},
			{Desc: "a pointer whose method set has Error implements, as a value", typ: types.NewPointer(es), implements: true},
			{Desc: "the struct behind that pointer does not implement error", typ: es},
			{Desc: "a type parameter constrained by error implements it", typ: constrained("E", errorType), implements: true},
			{Desc: "string is neither", typ: stringType},
		}) {
			gotest.Equal(it, row.isInterface, typeclass.IsErrorInterface(row.typ), "IsErrorInterface(%s)", row.typ)
			gotest.Equal(it, row.implements, typeclass.ImplementsError(row.typ), "ImplementsError(%s)", row.typ)
		}
	})
}
