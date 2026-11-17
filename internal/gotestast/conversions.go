package gotestast

import (
	"go/ast"
	"go/token"
	"go/types"
)

// convertedToInterface records, for each value converted to an interface
// type in root (an assignment, an argument, a return, a composite literal
// element, a send or an explicit conversion), the methods of that interface
// as reached on the value's named type: key "pkg.Type.Method".
func convertedToInterface(info *types.Info, root ast.Node, reached map[string]bool) {
	walkConversions(info, root, nil, func(expr ast.Expr, dst types.Type) {
		src := info.TypeOf(expr)
		if src == nil || dst == nil || !types.IsInterface(dst) || types.IsInterface(src) {
			return
		}
		n := namedBehind(src)
		iface, ok := dst.Underlying().(*types.Interface)
		if n == nil || !ok {
			return
		}
		for i := range iface.NumMethods() {
			reached[typeKey(n)+"."+iface.Method(i).Name()] = true
		}
	})
}

// walkConversions calls convert for every expression root hands to a slot
// of a known type; results are the enclosing function's.
func walkConversions(info *types.Info, root ast.Node, results *types.Tuple, convert func(ast.Expr, types.Type)) {
	ast.Inspect(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n != root {
				if n.Body != nil {
					walkConversions(info, n.Body, signatureResults(info.TypeOf(n.Name)), convert)
				}
				return false
			}
		case *ast.FuncLit:
			if n != root {
				walkConversions(info, n.Body, signatureResults(info.TypeOf(n)), convert)
				return false
			}
		case *ast.AssignStmt:
			if n.Tok == token.ASSIGN && len(n.Lhs) == len(n.Rhs) {
				for i := range n.Lhs {
					convert(n.Rhs[i], info.TypeOf(n.Lhs[i]))
				}
			}
		case *ast.ValueSpec:
			if n.Type != nil && len(n.Values) == len(n.Names) {
				for _, v := range n.Values {
					convert(v, info.TypeOf(n.Type))
				}
			}
		case *ast.ReturnStmt:
			if results != nil && len(n.Results) == results.Len() {
				for i, r := range n.Results {
					convert(r, results.At(i).Type())
				}
			}
		case *ast.SendStmt:
			if ch, ok := typeUnder(info.TypeOf(n.Chan)).(*types.Chan); ok {
				convert(n.Value, ch.Elem())
			}
		case *ast.CallExpr:
			callConversions(info, n, convert)
		case *ast.CompositeLit:
			literalConversions(info, n, convert)
		}
		return true
	})
}

func callConversions(info *types.Info, call *ast.CallExpr, convert func(ast.Expr, types.Type)) {
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
		if len(call.Args) == 1 {
			convert(call.Args[0], tv.Type)
		}
		return
	}
	sig, ok := typeUnder(info.TypeOf(call.Fun)).(*types.Signature)
	if !ok {
		return
	}
	params := sig.Params()
	for i, arg := range call.Args {
		switch {
		case sig.Variadic() && i >= params.Len()-1:
			last := params.At(params.Len() - 1).Type()
			if call.Ellipsis.IsValid() {
				convert(arg, last)
			} else if s, ok := last.Underlying().(*types.Slice); ok {
				convert(arg, s.Elem())
			}
		case i < params.Len():
			convert(arg, params.At(i).Type())
		}
	}
}

func literalConversions(info *types.Info, lit *ast.CompositeLit, convert func(ast.Expr, types.Type)) {
	switch u := typeUnder(info.TypeOf(lit)).(type) {
	case *types.Struct:
		for i, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					for j := range u.NumFields() {
						if u.Field(j).Name() == key.Name {
							convert(kv.Value, u.Field(j).Type())
						}
					}
				}
			} else if i < u.NumFields() {
				convert(elt, u.Field(i).Type())
			}
		}
	case *types.Slice:
		elementConversions(lit, nil, u.Elem(), convert)
	case *types.Array:
		elementConversions(lit, nil, u.Elem(), convert)
	case *types.Map:
		elementConversions(lit, u.Key(), u.Elem(), convert)
	}
}

func elementConversions(lit *ast.CompositeLit, key, elem types.Type, convert func(ast.Expr, types.Type)) {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key != nil {
				convert(kv.Key, key)
			}
			convert(kv.Value, elem)
			continue
		}
		convert(elt, elem)
	}
}

// typeUnder is t's underlying type through a pointer, for a composite
// literal written as &T{...}; nil for nil.
func typeUnder(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem().Underlying()
	}
	return t.Underlying()
}

func signatureResults(t types.Type) *types.Tuple {
	if sig, ok := t.(*types.Signature); ok {
		return sig.Results()
	}
	return nil
}
