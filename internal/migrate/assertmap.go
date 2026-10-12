package migrate

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

// mapAssertion is the gotest call that keeps a testify assertion's runtime
// behavior, judged by its arguments' types: gotest's Zero needs a
// comparable type, Empty counts only collections and NotNil rejects types
// that cannot be nil, where testify accepted anything. why explains a
// call no gotest assertion matches; name is then empty. args exclude t.
func (u *unit) mapAssertion(f *ast.File, name string, args []ast.Expr) (gotestName string, newArgs []ast.Expr, why string) {
	g, name, ok := gotestAssertion(name)
	if !ok {
		return "", nil, ""
	}
	if len(args) == 0 {
		return g, args, ""
	}
	if (name == "Equal" || name == "NotEqual") && len(args) >= 2 {
		return u.equalCall(f, g, args), args, ""
	}
	t := u.info.TypeOf(args[0])
	if t == nil {
		return g, args, "" // the type-check after the migration judges it
	}
	t = types.Unalias(t)
	if _, ok := t.(*types.TypeParam); ok && (name == "Zero" || name == "NotZero" || name == "Empty" || name == "NotEmpty" || name == "NotNil") {
		return "", nil, name + " on a value of type parameter " + t.String() + ": testify judged the dynamic type, gotest's assertion is decided by the type — convert by hand"
	}
	under := t.Underlying()
	_, isIface := under.(*types.Interface)
	ts := types.TypeString(t, types.RelativeTo(u.pkgOf(f)))

	switch name {
	case "Zero", "NotZero":
		switch {
		case isIface && name == "NotZero":
			return "", nil, "testify NotZero on interface type " + ts + " also fails for a zero value inside it; gotest.NotZero fails only for nil — use gotest.NotZero if non-nil is meant"
		case types.Comparable(t):
			return g, args, ""
		case isNilKind(under):
			return pick(name == "Zero", "Nil", "NotNil"), args, ""
		}
		return u.zeroCompare(f, name, args, t, ts)

	case "Empty", "NotEmpty":
		switch x := under.(type) {
		case *types.Slice, *types.Map, *types.Chan:
			return g, args, ""
		case *types.Basic:
			if x.Info()&types.IsString != 0 || x.Kind() == types.UntypedNil {
				return g, args, ""
			}
		case *types.Pointer:
			switch x.Elem().Underlying().(type) {
			case *types.Slice, *types.Map, *types.Chan:
				return g, args, ""
			}
			if b, ok := x.Elem().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
				return g, args, ""
			}
			return "", nil, "testify " + name + " on " + ts + " counts a pointer to a zero value as empty; gotest." + g + " looks only into collections — convert by hand"
		case *types.Signature:
			return pick(name == "Empty", "Nil", "NotNil"), args, ""
		case *types.Interface:
			if name == "Empty" {
				return g, args, ""
			}
			return "", nil, "testify NotEmpty on interface type " + ts + " counts a zero value inside it as empty; gotest.NotEmpty accepts only collections — convert by hand"
		}
		// Arrays, structs, numbers and bools are empty when zero.
		if types.Comparable(t) {
			return pick(name == "Empty", "Zero", "NotZero"), args, ""
		}
		return u.zeroCompare(f, name, args, t, ts)

	case "NotNil":
		if isIface || isNilKind(under) || isPointerish(under) {
			return g, args, ""
		}
		return "", nil, "NotNil on " + ts + " always passed in testify, as it can never be nil; gotest.NotNil fails on types that cannot be nil — delete it, or assert gotest.NotZero"
	}
	return g, args, ""
}

func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// isNilKind: slices, maps and funcs, which can be nil but not compared.
func isNilKind(t types.Type) bool {
	switch t.(type) {
	case *types.Slice, *types.Map, *types.Signature:
		return true
	}
	return false
}

// isPointerish: the comparable types that can be nil.
func isPointerish(t types.Type) bool {
	switch x := t.(type) {
	case *types.Pointer, *types.Chan:
		return true
	case *types.Basic:
		return x.Kind() == types.UnsafePointer || x.Kind() == types.UntypedNil
	}
	return false
}

// zeroCompare rewrites a zero check on a type gotest.Zero cannot take as a
// deep comparison with the type's zero value, which is what testify did,
// when the type can be spelled here.
func (u *unit) zeroCompare(f *ast.File, name string, args []ast.Expr, t types.Type, ts string) (string, []ast.Expr, string) {
	lit := u.zeroLit(f, t, args[0].Pos())
	if lit == nil {
		return "", nil, "testify " + name + " compared " + ts + " deeply with its zero value; gotest's Zero needs a comparable type — write gotest." + pick(name == "Zero" || name == "Empty", "Equal", "NotEqual") + " against its zero value"
	}
	return pick(name == "Zero" || name == "Empty", "Equal", "NotEqual"), append([]ast.Expr{lit}, args...), ""
}

// equalCall is gotest's Equal or NotEqual, instantiated with the type of
// the one argument the other is assignable to when their types differ:
// testify compared an interface and a concrete value, which gotest's
// single type parameter does not infer.
func (u *unit) equalCall(f *ast.File, g string, args []ast.Expr) string {
	a, b := u.info.TypeOf(args[0]), u.info.TypeOf(args[1])
	if a == nil || b == nil || u.untypedConst(args[0]) || u.untypedConst(args[1]) || types.Identical(a, b) {
		return g
	}
	var target types.Type
	switch {
	case types.AssignableTo(b, a):
		target = a
	case types.AssignableTo(a, b):
		target = b
	default:
		return g // the type-check after the migration marks it
	}
	if ts := u.typeString(f, target); ts != "" {
		return g + "[" + ts + "]"
	}
	return g
}

// untypedConst reports whether e is an untyped constant, which takes the
// type the other argument infers.
func (u *unit) untypedConst(e ast.Expr) bool {
	switch x := ast.Unparen(e).(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		if x.Name == "nil" {
			return true
		}
		c, ok := u.info.Uses[x].(*types.Const)
		if !ok {
			return false
		}
		b, ok := c.Type().(*types.Basic)
		return ok && b.Info()&types.IsUntyped != 0
	case *ast.UnaryExpr:
		return u.untypedConst(x.X)
	case *ast.BinaryExpr:
		return u.untypedConst(x.X) && u.untypedConst(x.Y)
	}
	return false
}

// typeString spells t as f names it, empty when a package t needs is not
// imported there.
func (u *unit) typeString(f *ast.File, t types.Type) string {
	ok := true
	s := types.TypeString(t, func(p *types.Package) string {
		if p == u.pkgOf(f) {
			return ""
		}
		name := u.importName(f, p.Path())
		ok = ok && name != ""
		return name
	})
	if !ok {
		return ""
	}
	return s
}

// importName is the name f imports path under, empty when it does not.
func (u *unit) importName(f *ast.File, path string) string {
	for _, is := range f.Imports {
		if p, _ := strconv.Unquote(is.Path.Value); p != path {
			continue
		}
		var pn types.Object
		if is.Name != nil {
			pn = u.info.Defs[is.Name]
		} else {
			pn = u.info.Implicits[is]
		}
		if pn != nil && pn.Name() != "_" && pn.Name() != "." {
			return pn.Name()
		}
	}
	return ""
}

// zeroLit is T{} for a named struct or array type t this file can name at
// pos, nil otherwise.
func (u *unit) zeroLit(f *ast.File, t types.Type, pos token.Pos) ast.Expr {
	n, ok := t.(*types.Named)
	if !ok || n.TypeArgs().Len() > 0 {
		return nil
	}
	switch n.Underlying().(type) {
	case *types.Struct, *types.Array:
	default:
		return nil
	}
	obj := n.Obj()
	if obj.Pkg() == u.pkgOf(f) {
		if u.lookup(f, obj.Name(), pos) != obj {
			return nil // shadowed here
		}
		return &ast.CompositeLit{Type: ast.NewIdent(obj.Name())}
	}
	name := u.importName(f, obj.Pkg().Path())
	if _, ok := u.lookup(f, name, pos).(*types.PkgName); !ok || !obj.Exported() {
		return nil
	}
	return &ast.CompositeLit{Type: &ast.SelectorExpr{X: ast.NewIdent(name), Sel: ast.NewIdent(obj.Name())}}
}

// pkgOf is the package f belongs to.
func (u *unit) pkgOf(f *ast.File) *types.Package { return u.filePkg[f] }

// lookup resolves name at pos in f, nil when nothing declares it.
func (u *unit) lookup(f *ast.File, name string, pos token.Pos) types.Object {
	scope := u.info.Scopes[f]
	if scope == nil {
		return nil
	}
	if inner := scope.Innermost(pos); inner != nil {
		scope = inner
	}
	_, obj := scope.LookupParent(name, pos)
	return obj
}
