package gotestast

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// harnessBuiltins are the built-in functions the generated harness calls.
var harnessBuiltins = map[string]bool{"append": true, "len": true, "make": true, "panic": true}

// PredeclaredClash returns the position of a package-level declaration that
// takes a name Go predeclares and the generated harness uses with Go's
// meaning, and the error it is refused with. An alias that means the same
// type, such as any = interface{}, is no clash; a name the harness does not
// use, such as min, is none either.
func PredeclaredClash(pkg *packages.Package) (token.Pos, error) {
	if pkg == nil || pkg.Types == nil {
		return token.NoPos, nil
	}
	scope := pkg.Types.Scope()
	names := scope.Names()
	sort.Strings(names)
	for _, name := range names {
		universal := types.Universe.Lookup(name)
		if universal == nil {
			continue
		}
		obj := scope.Lookup(name)
		switch u := universal.(type) {
		case *types.TypeName:
			if tn, ok := obj.(*types.TypeName); ok && types.Identical(tn.Type(), u.Type()) {
				continue
			}
		case *types.Builtin:
			if !harnessBuiltins[name] {
				continue
			}
		case *types.Const, *types.Nil:
			if name == "iota" {
				continue
			}
		default:
			continue
		}
		return obj.Pos(), fmt.Errorf("package %s declares %s, a name Go predeclares that the generated harness uses; rename it", pkg.Types.Name(), name)
	}
	return token.NoPos, nil
}
