// Package typeclass classifies Go types the one way every part of gotest
// reads them: discovery, the generator and the lint rules decide nilability,
// comparability and error-ness here, so no two of them can disagree.
//
// Aliases are resolved first. A type parameter is read through its core type
// — the one underlying type its type set shares — and a parameter with no core
// type belongs to no class.
package typeclass

import "go/types"

var errorType = types.Universe.Lookup("error").Type()

// CoreType returns the type t classifies as: its underlying type, or for a
// type parameter the single underlying type every type in its type set has.
// It is nil for a type parameter whose type set has no such type (any,
// comparable, int | float64) or is empty.
func CoreType(t types.Type) types.Type {
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
	hasTerms := terms(iface, func(term types.Type) {
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

// terms visits every type term of iface's type set, through embedded unions
// and interfaces, and reports whether it holds any term at all (an interface
// of methods alone constrains to every type).
func terms(iface *types.Interface, visit func(types.Type)) bool {
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
				found = terms(inner, visit) || found
				continue
			}
			visit(e)
			found = true
		}
	}
	return found
}

// Nilable reports whether nil is a value of t: pointers, slices, maps,
// channels, functions, interfaces and unsafe.Pointer, named or not.
func Nilable(t types.Type) bool {
	core := CoreType(t)
	if core == nil {
		return false
	}
	switch u := core.(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	}
	return false
}

// Comparable reports whether values of t may be compared with ==: for a type
// parameter, whether its constraint guarantees it.
func Comparable(t types.Type) bool {
	return types.Comparable(types.Unalias(t))
}

// IsErrorInterface reports whether t is the error interface: error itself, an
// alias of it, or an interface with exactly its method set. A type parameter
// constrained by error is some type implementing it, not the interface.
func IsErrorInterface(t types.Type) bool {
	core := CoreType(t)
	return core != nil && types.Identical(core, errorType.Underlying())
}

// ImplementsError reports whether a value of t is assignable to error.
func ImplementsError(t types.Type) bool {
	return types.AssignableTo(types.Unalias(t), errorType)
}
