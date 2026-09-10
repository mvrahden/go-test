package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/tools/go/packages"
)

// FuzzCall is the f.Fuzz call of one suite fuzz method: the generated
// wrapper it belongs to and the callback's parameter types after the
// leading *gotest.T, in engine order. The types come from the argument
// expression's static type, so the callback may be a literal, a method
// value, or a named function.
type FuzzCall struct {
	FuncName string // generated wrapper name, e.g. "FuzzUserTestSuite_FuzzCreate"
	Types    []types.Type
	Pos      token.Pos // position of the f.Fuzz call
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
		return !ok || eb.Kind() != types.Uint8
	}
	return false
}

// IsFFuzzCall reports whether ce is a call of (*gotest.F).Fuzz.
func IsFFuzzCall(pkg *packages.Package, ce *ast.CallExpr) bool {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Fuzz" {
		return false
	}
	recv := pkg.TypesInfo.TypeOf(sel.X)
	if recv == nil {
		return false
	}
	name := recv.String()
	return strings.HasPrefix(name, "*"+about.Repo) && strings.HasSuffix(name, "/gotest.F")
}

// CollectFuzzCalls finds the f.Fuzz call of every suite fuzz method and
// returns them in (suite, method) order. A method that never calls f.Fuzz,
// calls it more than once, or hands it something other than a
// func(*gotest.T, ...) is an error naming the method — the engine would
// only fail later, with no pointer at the source.
func CollectFuzzCalls(pkg *packages.Package, suites TestSuiteSpecSet) ([]FuzzCall, error) {
	if pkg == nil || pkg.TypesInfo == nil || len(suites) == 0 {
		return nil, nil
	}

	var out []FuzzCall
	for _, ts := range suites {
		for _, fz := range ts.Fuzzers() {
			decl, ok := fz.n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			method := ts.Identifier() + "." + fz.Identifier()
			funcName := fmt.Sprintf("Fuzz%s_%s", ts.Identifier(), fz.Identifier())
			var calls []*ast.CallExpr
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				if ce, ok := n.(*ast.CallExpr); ok && IsFFuzzCall(pkg, ce) {
					calls = append(calls, ce)
				}
				return true
			})
			switch len(calls) {
			case 0:
				return nil, fmt.Errorf("fuzz method %s never calls f.Fuzz", method)
			case 1:
			default:
				return nil, fmt.Errorf("fuzz method %s calls f.Fuzz more than once — one target per method", method)
			}
			ce := calls[0]
			if len(ce.Args) != 1 {
				return nil, fmt.Errorf("fuzz method %s: f.Fuzz takes exactly one callback", method)
			}
			typs, err := fuzzCallbackTypes(pkg.TypesInfo.TypeOf(ce.Args[0]))
			if err != nil {
				return nil, fmt.Errorf("fuzz method %s: %w", method, err)
			}
			out = append(out, FuzzCall{FuncName: funcName, Types: typs, Pos: ce.Pos()})
		}
	}
	return out, nil
}

// fuzzCallbackTypes validates a callback type and returns its parameters
// after the leading *gotest.T.
func fuzzCallbackTypes(t types.Type) ([]types.Type, error) {
	sig, ok := types.Unalias(t).(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("f.Fuzz callback must be a func(*gotest.T, ...), got %s", types.TypeString(t, nil))
	}
	params := sig.Params()
	if params.Len() == 0 || !isGotestTPtr(params.At(0).Type()) {
		return nil, fmt.Errorf("f.Fuzz callback's first parameter must be *gotest.T")
	}
	if sig.Results().Len() != 0 {
		return nil, fmt.Errorf("f.Fuzz callback must not return anything")
	}
	if sig.Variadic() {
		return nil, fmt.Errorf("f.Fuzz callback must not be variadic")
	}
	typs := make([]types.Type, 0, params.Len()-1)
	for i := 1; i < params.Len(); i++ {
		typs = append(typs, params.At(i).Type())
	}
	return typs, nil
}

func isGotestTPtr(t types.Type) bool {
	name := t.String()
	return strings.HasPrefix(name, "*"+about.Repo) && strings.HasSuffix(name, "/gotest.T")
}
