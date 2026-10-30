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
		if why := u.constMismatch(f, name, g, args); why != "" {
			return "", nil, why
		}
		call, why := u.equalCall(f, name, g, args)
		if why != "" {
			return "", nil, why
		}
		return call, args, ""
	}
	t := u.info.TypeOf(args[0])
	if t == nil {
		return g, args, "" // the type-check after the migration judges it
	}
	spelled := t
	t = types.Unalias(t)
	if _, ok := t.(*types.TypeParam); ok && (name == "Zero" || name == "NotZero" || name == "Empty" || name == "NotEmpty" || name == "NotNil") {
		return "", nil, name + " on a value of type parameter " + t.String() + ": testify judged the dynamic type, gotest's assertion is decided by the type — convert by hand"
	}
	under := t.Underlying()
	_, isIface := under.(*types.Interface)
	ts := types.TypeString(spelled, types.RelativeTo(u.pkgOf(f)))

	switch name {
	case "ElementsMatch", "Subset":
		for _, a := range args[:min(len(args), 2)] {
			if el := elemType(u.info.TypeOf(a)); el != nil && !deepAsEqual(el, map[types.Type]bool{}) {
				es := types.TypeString(el, types.RelativeTo(u.pkgOf(f)))
				return "", nil, "testify " + name + " compared the " + es + " elements deeply; gotest." + g + " compares them with ==, so distinct pointers or interfaces holding equal values no longer match — compare the values by hand"
			}
		}

	case "Zero", "NotZero":
		switch {
		case isIface && name == "Zero":
			return "", nil, "testify Zero on interface type " + ts + " also passes for a zero value inside it, a typed nil included; gotest.Zero passes only for nil — use gotest.Zero if nil is meant"
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
				return "", nil, "testify Empty on interface type " + ts + " judges the value inside it, so a zero number or struct counts as empty; gotest.Empty accepts only collections and pointers — convert by hand"
			}
			return "", nil, "testify NotEmpty on interface type " + ts + " counts a zero value inside it as empty; gotest.NotEmpty accepts only collections — convert by hand"
		}
		// Arrays, structs, numbers and bools are empty when zero.
		if types.Comparable(t) {
			return pick(name == "Empty", "Zero", "NotZero"), args, ""
		}
		return u.zeroCompare(f, name, args, t, ts)

	case "Len":
		if p, ok := under.(*types.Pointer); ok {
			if _, ok := p.Elem().Underlying().(*types.Array); ok {
				return "", nil, "testify Len on " + ts + " counted the array's length, nil pointer or not; gotest.Len takes no pointer — assert on len(" + types.ExprString(args[0]) + ") with gotest.Equal"
			}
		}

	case "InDelta":
		if len(args) >= 3 {
			if why := u.nanDelta(args); why != "" {
				return "", nil, why
			}
		}

	case "NotNil":
		if isIface {
			// No gotest assertion judges the value inside as testify did.
			return "", nil, "testify NotNil on interface type " + ts + " fails a typed nil inside it, which gotest.NotZero passes, and passes a non-nilable value inside it, which gotest.NotNil fails — use gotest.NotNil if the value is nilable, gotest.NotZero if only nil is meant"
		}
		if isNilKind(under) || isPointerish(under) {
			return g, args, ""
		}
		return "", nil, "NotNil on " + ts + " always passed in testify, as it can never be nil; gotest.NotNil fails on types that cannot be nil — delete it, or assert gotest.NotZero"
	}
	return g, args, ""
}

// nanDelta explains an InDelta whose verdict depends on a NaN, which
// testify judged otherwise: it passed two NaN values and any difference
// within a NaN delta, where gotest.InDelta fails both. Empty when no
// argument can be NaN: a constant or an integer cannot.
func (u *unit) nanDelta(args []ast.Expr) string {
	mayBeNaN := func(e ast.Expr) bool {
		tv, ok := u.info.Types[e]
		if !ok || tv.Value != nil {
			return false
		}
		b, ok := tv.Type.Underlying().(*types.Basic)
		return !ok || b.Info()&types.IsInteger == 0
	}
	switch {
	case mayBeNaN(args[2]):
		return "testify InDelta passed any difference within a NaN delta; gotest.InDelta fails a NaN delta — convert by hand"
	case mayBeNaN(args[0]) && mayBeNaN(args[1]):
		return "testify InDelta passed when both values were NaN; gotest.InDelta fails then — check math.IsNaN for both by hand if NaN is expected, else use gotest.InDelta"
	}
	return ""
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

// equalCall is gotest's Equal or NotEqual, instantiated with the interface
// type the other argument is assignable to when their types differ: testify
// compared an interface and a concrete value, which gotest's single type
// parameter does not infer. Two concrete types testify never found equal,
// which an instantiation would convert, are explained in why instead.
func (u *unit) equalCall(f *ast.File, name, g string, args []ast.Expr) (call, why string) {
	a, b := u.info.TypeOf(args[0]), u.info.TypeOf(args[1])
	if a == nil || b == nil || u.untypedConst(args[0]) || u.untypedConst(args[1]) || types.Identical(a, b) {
		return g, ""
	}
	var target types.Type
	switch {
	case types.AssignableTo(b, a):
		target = a
	case types.AssignableTo(a, b):
		target = b
	default:
		return g, "" // the type-check after the migration marks it
	}
	if !types.IsInterface(target) {
		qual := types.RelativeTo(u.pkgOf(f))
		as, bs := types.TypeString(a, qual), types.TypeString(b, qual)
		return "", "testify " + name + " compared a " + as + " with a " + bs + ", types it never finds equal; gotest." + g + " would convert one to the other and compare the values — convert by hand"
	}
	if ts := u.typeString(f, target); ts != "" {
		return g + "[" + ts + "]", ""
	}
	return g, ""
}

// constMismatch explains an Equal or NotEqual whose untyped constant
// testify compared as its default type with a value of another, which it
// never finds equal, where gotest's Equal[V] converts the constant to
// that type and compares values. Empty when no constant does.
func (u *unit) constMismatch(f *ast.File, name, g string, args []ast.Expr) string {
	qual := types.RelativeTo(u.pkgOf(f))
	for i := range 2 {
		c, other := args[i], args[1-i]
		if !u.untypedConst(c) {
			continue
		}
		ct, ot := u.info.TypeOf(c), u.info.TypeOf(other)
		if ct == nil || ot == nil {
			return ""
		}
		ct, ot = types.Default(ct), types.Default(ot)
		if _, ok := ot.Underlying().(*types.Interface); ok || isUntypedNil(ot) || types.Identical(ct, ot) {
			continue
		}
		os := types.TypeString(ot, qual)
		if id, ok := ast.Unparen(c).(*ast.Ident); ok {
			if _, ok := u.info.Uses[id].(*types.Nil); ok {
				return "testify " + name + " compared nil with a value of type " + os + " held in an interface, which is never nil; gotest." + g + " compares that " + os + " itself with nil — convert by hand"
			}
		}
		cs := types.ExprString(c)
		return "testify " + name + " compared " + cs + " as " + types.TypeString(ct, qual) + " with a value of type " + os + ", types it never finds equal; gotest." + g + " converts " + cs + " to " + os + " and compares the values — convert by hand"
	}
	return ""
}

func isUntypedNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

// elemType is the element type of a slice or array type, nil for others.
func elemType(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	switch x := t.Underlying().(type) {
	case *types.Slice:
		return x.Elem()
	case *types.Array:
		return x.Elem()
	}
	return nil
}

// deepAsEqual reports whether == on values of t agrees with
// reflect.DeepEqual, which testify's ObjectsAreEqual is: no pointer or
// interface inside, where == compares identity and DeepEqual the values.
func deepAsEqual(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return true
	}
	seen[t] = true
	switch x := t.Underlying().(type) {
	case *types.Basic:
		return x.Kind() != types.UnsafePointer
	case *types.Chan:
		return true // DeepEqual compares channels with == too
	case *types.Array:
		return deepAsEqual(x.Elem(), seen)
	case *types.Struct:
		for i := range x.NumFields() {
			if !deepAsEqual(x.Field(i).Type(), seen) {
				return false
			}
		}
		return true
	}
	return false
}

// untypedConst reports whether e is an untyped constant, which takes the
// type the other argument infers: a constant whose operands are all
// untyped, nil included.
func (u *unit) untypedConst(e ast.Expr) bool {
	e = ast.Unparen(e)
	if id, ok := e.(*ast.Ident); ok {
		if _, ok := u.info.Uses[id].(*types.Nil); ok {
			return true
		}
	}
	if tv, ok := u.info.Types[e]; !ok || tv.Value == nil {
		return false
	}
	switch x := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return untypedObj(u.info.Uses[x])
	case *ast.SelectorExpr:
		return untypedObj(u.info.Uses[x.Sel])
	case *ast.UnaryExpr:
		return u.untypedConst(x.X)
	case *ast.BinaryExpr:
		if x.Op == token.SHL || x.Op == token.SHR {
			return u.untypedConst(x.X)
		}
		return u.untypedConst(x.X) && u.untypedConst(x.Y)
	}
	return false
}

// untypedObj reports whether obj is a constant declared without a type.
func untypedObj(obj types.Object) bool {
	c, ok := obj.(*types.Const)
	if !ok {
		return false
	}
	b, ok := c.Type().(*types.Basic)
	return ok && b.Info()&types.IsUntyped != 0
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
