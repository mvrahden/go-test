package lint

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// A rewrite is offered only where the new assertion's verdict equals the old
// one's on every value the operand's type can hold. These helpers read the
// operand's type the way the assertions read its values.

// coreType is t's underlying type, or for a type parameter the one underlying
// type its whole type set shares; nil when there is none (any, error,
// int | float64): the values decide, and no rewrite fits them all.
func coreType(t types.Type) types.Type {
	t = types.Unalias(t)
	tp, ok := t.(*types.TypeParam)
	if !ok {
		return t.Underlying()
	}
	iface, ok := types.Unalias(tp.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	var core types.Type
	shared := true
	hasTerms := typeTerms(iface, func(term types.Type) {
		u := types.Unalias(term).Underlying()
		switch {
		case core == nil:
			core = u
		case !types.Identical(core, u):
			shared = false
		}
	})
	if !hasTerms || !shared {
		return nil
	}
	return core
}

// typeTerms visits the type terms of iface, through embedded unions and
// interfaces, and reports whether there is any.
func typeTerms(iface *types.Interface, visit func(types.Type)) bool {
	found := false
	for i := range iface.NumEmbeddeds() {
		switch e := types.Unalias(iface.EmbeddedType(i)).(type) {
		case *types.Union:
			for j := range e.Len() {
				visit(e.Term(j).Type())
			}
			found = true
		default:
			if inner, ok := e.Underlying().(*types.Interface); ok {
				found = typeTerms(inner, visit) || found
				continue
			}
			visit(e)
			found = true
		}
	}
	return found
}

// nilKind is how a nil check reads an operand's type.
type nilKind int

const (
	unknownNil      nilKind = iota // a type parameter with no core type: its values decide
	notNilable                     // nil is no value of it
	errorIface                     // the error interface
	otherIface                     // any other interface
	comparableNil                  // pointer, channel, unsafe.Pointer
	uncomparableNil                // slice, map, func
)

var errorType = types.Universe.Lookup("error").Type()

// nilKindOf classifies expr's type. Only an interface compares to nil as
// itself: a nil *T converted to error is not nil, and Nil and Empty look
// through an interface to the nil pointer it holds.
func nilKindOf(pass *analysis.Pass, expr ast.Expr) nilKind {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return notNilable
	}
	core := coreType(t)
	switch u := core.(type) {
	case nil:
		return unknownNil
	case *types.Interface:
		if types.Identical(u, errorType.Underlying()) {
			return errorIface
		}
		return otherIface
	case *types.Pointer, *types.Chan:
		return comparableNil
	case *types.Slice, *types.Map, *types.Signature:
		return uncomparableNil
	case *types.Basic:
		if u.Kind() == types.UnsafePointer {
			return comparableNil
		}
	}
	return notNilable
}

// mayBeFloat reports whether an ordering operand may hold a float. The
// ordering assertions use cmp.Compare, which puts NaN below every number,
// so they disagree with the operators there.
func mayBeFloat(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return true
	}
	b, ok := coreType(t).(*types.Basic)
	if !ok {
		return true
	}
	return b.Info()&types.IsFloat != 0
}

// mayBeEmptyString reports whether a Contains needle may be the empty
// string, which Contains finds in an empty haystack too.
func mayBeEmptyString(pass *analysis.Pass, needle ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[needle]
	if !ok || tv.Type == nil {
		return true
	}
	if tv.Value != nil {
		return tv.Value.Kind() == constant.String && constant.StringVal(tv.Value) == ""
	}
	switch u := coreType(tv.Type).(type) {
	case nil, *types.Interface:
		return true
	case *types.Basic:
		return u.Info()&types.IsString != 0
	}
	return false
}

// hasTypeArgs reports whether an assertion is called with explicit type
// arguments, which may convert its operands before it reads them.
func hasTypeArgs(fun ast.Expr) bool {
	switch ast.Unparen(fun).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	}
	return false
}
