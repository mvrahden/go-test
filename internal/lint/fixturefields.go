package lint

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
)

// Fixture-field discovery shared by the suite-scoped fixture rules: the
// syntactic layer finds a suite's declared fixture-typed pointer fields,
// the type layer expands them into the DAG-closure of named fixture types
// the generator would wire for that suite.

// structFixtureFieldNames returns the names of typ's (a suite's
// TypeSpec.Type) fields that are typed as a pointer to something whose
// name ends in "Fixture" (which also covers the "SharedFixture" naming
// convention, since that suffix ends in "Fixture" too). An embedded
// fixture field (no explicit name) is keyed under its type name, since
// that is how Go addresses it (s.CacheFixture). Returns nil if typ isn't a
// struct or has no fixture-typed fields.
func structFixtureFieldNames(typ ast.Expr) map[string]bool {
	st, ok := typ.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	var fields map[string]bool
	for _, field := range st.Fields.List {
		typeName := fixtureFieldTypeName(field.Type)
		if typeName == "" {
			continue
		}
		if fields == nil {
			fields = map[string]bool{}
		}
		if len(field.Names) == 0 {
			fields[typeName] = true
			continue
		}
		for _, name := range field.Names {
			fields[name.Name] = true
		}
	}
	return fields
}

// fixtureFieldTypeName returns the name of the pointed-to type if expr is
// a pointer to something whose name ends in "Fixture", or "" otherwise.
func fixtureFieldTypeName(expr ast.Expr) string {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return ""
	}

	var name string
	switch t := star.X.(type) {
	case *ast.Ident:
		name = t.Name
	case *ast.SelectorExpr:
		name = t.Sel.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			name = id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			name = id.Name
		}
	}

	if !strings.HasSuffix(name, protocol.SuffixFixture) {
		return ""
	}
	return name
}

// fixtureNamedType returns the pointed-to named type when t is a pointer
// to a named type whose name ends in "Fixture" ("SharedFixture" included),
// or nil otherwise.
func fixtureNamedType(t types.Type) *types.Named {
	ptr, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return nil
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	if !ok || !strings.HasSuffix(named.Obj().Name(), protocol.SuffixFixture) {
		return nil
	}
	return named
}

// declaredFixtureClosure expands a suite's declared fixture fields into
// the DAG-closure of named fixture types reachable from them: each
// declared pointer field named in declared, then every fixture-typed
// field of those fixtures' structs, transitively — the same walk the
// resolver performs when it wires a suite's required fixtures.
func declaredFixtureClosure(st *types.Struct, declared map[string]bool) map[*types.TypeName]bool {
	closure := map[*types.TypeName]bool{}
	var walk func(s *types.Struct, keep map[string]bool)
	walk = func(s *types.Struct, keep map[string]bool) {
		for i := 0; i < s.NumFields(); i++ {
			f := s.Field(i)
			if keep != nil && !keep[f.Name()] {
				continue
			}
			named := fixtureNamedType(f.Type())
			if named == nil {
				continue
			}
			tn := named.Obj()
			if closure[tn] {
				continue
			}
			closure[tn] = true
			if inner, ok := named.Underlying().(*types.Struct); ok {
				walk(inner, nil)
			}
		}
	}
	walk(st, declared)
	return closure
}
