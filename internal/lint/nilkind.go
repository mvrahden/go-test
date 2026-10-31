package lint

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/mvrahden/go-test/internal/typeclass"
)

// A rewrite is offered only where the new assertion's verdict equals the old
// one's on every value the operand's type can hold. These helpers read the
// operand's type the way the assertions read its values, through typeclass.

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

// nilKindOf classifies expr's type. Only an interface compares to nil as
// itself: a nil *T converted to error is not nil, and Nil and Empty look
// through an interface to the nil pointer it holds.
func nilKindOf(pass *analysis.Pass, expr ast.Expr) nilKind {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return notNilable
	}
	core := typeclass.CoreType(t)
	switch {
	case core == nil:
		return unknownNil
	case !typeclass.Nilable(t):
		return notNilable
	case typeclass.IsErrorInterface(t):
		return errorIface
	case isInterfaceType(t):
		return otherIface
	case typeclass.Comparable(core):
		return comparableNil
	}
	return uncomparableNil
}

// mayBeFloat reports whether an ordering operand may hold a float. The
// ordering assertions use cmp.Compare, which puts NaN below every number,
// so they disagree with the operators there. A type parameter over several
// types may hold one.
func mayBeFloat(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return true
	}
	b, ok := typeclass.CoreType(t).(*types.Basic)
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
	switch u := typeclass.CoreType(tv.Type).(type) {
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
