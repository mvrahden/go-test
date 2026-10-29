package refactor

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
)

// Object names a package-level type, or a method of one, the same way in
// every type-check of its package: the package itself, its test variant,
// and the external test package that imports it.
type Object struct {
	PkgPath string
	Type    string // the type, or the method's receiver type
	Method  string // empty for the type itself
}

// ObjectOf names obj, false when obj is neither a package-level type nor a
// method of one.
func ObjectOf(obj types.Object) (Object, bool) {
	if obj == nil || obj.Pkg() == nil {
		return Object{}, false
	}
	switch o := obj.(type) {
	case *types.TypeName:
		if o.Pkg().Scope().Lookup(o.Name()) != o {
			return Object{}, false
		}
		return Object{PkgPath: o.Pkg().Path(), Type: o.Name()}, true
	case *types.Func:
		recv := o.Origin().Signature().Recv()
		if recv == nil {
			return Object{}, false
		}
		t := recv.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		named, ok := types.Unalias(t).(*types.Named)
		if !ok {
			return Object{}, false
		}
		base := named.Origin().Obj()
		if base.Pkg() == nil || base.Pkg().Scope().Lookup(base.Name()) != base {
			return Object{}, false
		}
		return Object{PkgPath: base.Pkg().Path(), Type: base.Name(), Method: o.Name()}, true
	}
	return Object{}, false
}

// Name is the identifier that spells the object.
func (o Object) Name() string {
	if o.Method != "" {
		return o.Method
	}
	return o.Type
}

// DeclaredAt names the object whose name is declared at pos.
func DeclaredAt(info *types.Info, pos token.Pos) (Object, bool) {
	for id, obj := range info.Defs {
		if id.Pos() == pos {
			return ObjectOf(obj)
		}
	}
	return Object{}, false
}

// Idents returns, in source order, the identifiers in files that refer to
// target: its declaration, the receivers of its methods and every other
// use, package-qualified ones included. A use counts by the object it
// resolves to, never by its spelling, so a same-named identifier in another
// scope or package is left out. Generated files are skipped: they are
// regenerated, not edited.
func Idents(files []*ast.File, info *types.Info, target Object) []*ast.Ident {
	var out []*ast.Ident
	for _, file := range files {
		if ast.IsGenerated(file) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || id.Name != target.Name() {
				return true
			}
			// An embedded field's name is in both: Defs holds the field,
			// Uses the type.
			for _, obj := range []types.Object{info.Defs[id], info.Uses[id]} {
				if refersTo(obj, target) {
					out = append(out, id)
					break
				}
			}
			return true
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pos() < out[j].Pos() })
	return out
}

// refersTo reports whether obj is target, or a field that embeds target and
// is named by it: selectors and literal keys spell such a field.
func refersTo(obj types.Object, target Object) bool {
	if o, ok := ObjectOf(obj); ok {
		return o == target
	}
	v, ok := obj.(*types.Var)
	if !ok || !v.Embedded() || target.Method != "" {
		return false
	}
	t := v.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	// An alias names the field itself, so only the named type counts.
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	o, ok := ObjectOf(named.Origin().Obj())
	return ok && o == target
}
