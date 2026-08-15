package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// FuzzArg is one instantiated type argument of a gotest.Fuzz/Fuzz2/Fuzz3
// call inside a suite fuzz method.
//
// It is read from types.Info.Instances — the instantiation, never the
// callback body — so the callback does not have to be an inline function
// literal: a method value or a named function works just as well.
type FuzzArg struct {
	FuncName string     // generated wrapper name, e.g. "FuzzUserTestSuite_FuzzCreate"
	Adapter  string     // "Fuzz", "Fuzz2" or "Fuzz3"
	Index    int        // 0-based position among the fuzzed arguments
	Type     types.Type // the instantiated type argument
	Pos      token.Pos  // position of the adapter call
}

// NativeFuzzType reports whether Go's fuzzing engine accepts t directly.
// The set is exactly the fifteen types testing.F.Fuzz allows; a named type
// over one of them does NOT qualify (testing matches on reflect.Type
// identity), which is why "type Age int" needs a codec just as a struct
// does. This is the single source of truth for the native set — the codec
// emitter and the lint rules both key off it, so they can never disagree
// about which targets are codec-backed.
func NativeFuzzType(t types.Type) bool {
	switch u := types.Unalias(t).(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.String, types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
	case *types.Slice:
		eb, ok := types.Unalias(u.Elem()).(*types.Basic)
		return ok && eb.Kind() == types.Uint8
	}
	return false
}

// PassthroughFuzzType reports whether gotest hands an argument of type t to
// the engine exactly as declared: the unnamed string, bool, and []byte, and
// nothing else. Every other type — a struct, a named type, a plain number —
// gets a generated fan. Numbers fan on purpose: as fixed-width []byte leaves
// they get the engine's richest mutator instead of its poorest (see the
// leaf encoding policy in docs/design/fuzz-structs.md).
func PassthroughFuzzType(t types.Type) bool {
	t = types.Unalias(t)
	if _, isNamed := t.(*types.Named); isNamed {
		return false
	}
	switch u := t.(type) {
	case *types.Basic:
		return u.Kind() == types.String || u.Kind() == types.Bool
	case *types.Slice:
		eb, ok := types.Unalias(u.Elem()).(*types.Basic)
		return ok && eb.Kind() == types.Uint8
	}
	return false
}

// FuzzCorpusShapeBound reports whether corpus entries for a fuzz argument
// of type t depend on a field layout — a struct, pointer, array, or
// non-byte slice, whose fanned positions follow declaration order. A
// same-kind field reorder silently reinterprets such entries and an added
// or removed field rejects them; a scalar or byte slice has no layout to
// drift.
func FuzzCorpusShapeBound(t types.Type) bool {
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Struct, *types.Pointer, *types.Array:
		return true
	case *types.Slice:
		eb, ok := types.Unalias(u.Elem()).(*types.Basic)
		return !(ok && eb.Kind() == types.Uint8)
	}
	return false
}

// CollectFuzzArgs walks every suite fuzz method's body for gotest.Fuzz
// adapter calls and returns their type arguments in deterministic
// (suite, method, call, argument) order. Returns nil when the package fuzzes
// nothing.
func CollectFuzzArgs(pkg *packages.Package, suites TestSuiteSpecSet) []FuzzArg {
	if pkg == nil || pkg.TypesInfo == nil || len(suites) == 0 {
		return nil
	}

	var out []FuzzArg
	for _, ts := range suites {
		for _, fz := range ts.Fuzzers() {
			decl, ok := fz.n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			funcName := fmt.Sprintf("Fuzz%s_%s", ts.Identifier(), fz.Identifier())
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				fn := calleeFuncOf(pkg, ce.Fun)
				if fn == nil || !isGotestFuzzAdapter(fn) {
					return true
				}
				ident := calleeIdentOf(ce.Fun)
				if ident == nil {
					return true
				}
				inst, ok := pkg.TypesInfo.Instances[ident]
				if !ok || inst.TypeArgs == nil {
					return true
				}
				for i := 0; i < inst.TypeArgs.Len(); i++ {
					out = append(out, FuzzArg{
						FuncName: funcName,
						Adapter:  fn.Name(),
						Index:    i,
						Type:     inst.TypeArgs.At(i),
						Pos:      ce.Pos(),
					})
				}
				return true
			})
		}
	}
	return out
}
