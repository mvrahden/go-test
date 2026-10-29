package gotestast

import "go/types"

// instanceArgs maps the type parameters of m's generic receiver to the type
// arguments the alias ts instantiates them with. It is nil for a suite that
// is no generic alias.
func instanceArgs(ts *TestSuiteSpec, m *TestSuiteMethod) map[*types.TypeParam]types.Type {
	if !ts.IsGenericAlias() || m.sig == nil {
		return nil
	}
	named, ok := types.Unalias(ts.pkg.TypesInfo.TypeOf(ts.ts.Type)).(*types.Named)
	if !ok {
		return nil
	}
	params, args := m.sig.RecvTypeParams(), named.TypeArgs()
	if params == nil || args == nil || params.Len() != args.Len() {
		return nil
	}
	out := make(map[*types.TypeParam]types.Type, params.Len())
	for i := range params.Len() {
		out[params.At(i)] = args.At(i)
	}
	return out
}

// substitute replaces the type parameters in t by their arguments.
func substitute(t types.Type, args map[*types.TypeParam]types.Type) types.Type {
	if len(args) == 0 {
		return t
	}
	switch x := t.(type) {
	case *types.TypeParam:
		if a, ok := args[x]; ok {
			return a
		}
	case *types.Pointer:
		return types.NewPointer(substitute(x.Elem(), args))
	case *types.Slice:
		return types.NewSlice(substitute(x.Elem(), args))
	case *types.Array:
		return types.NewArray(substitute(x.Elem(), args), x.Len())
	case *types.Map:
		return types.NewMap(substitute(x.Key(), args), substitute(x.Elem(), args))
	case *types.Named:
		targs := x.TypeArgs()
		if targs == nil || targs.Len() == 0 {
			return t
		}
		subst := make([]types.Type, targs.Len())
		for i := range targs.Len() {
			subst[i] = substitute(targs.At(i), args)
		}
		if inst, err := types.Instantiate(nil, x.Origin(), subst, false); err == nil {
			return inst
		}
	}
	return t
}
